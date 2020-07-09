package bridge

import (
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/sirupsen/logrus"
)

var serviceIDPattern = regexp.MustCompile(`^\[(.+?)\]:([a-zA-Z0-9][a-zA-Z0-9_.-]+):[0-9]+$`)

var log = logrus.WithField("component", "bridge")

// Bridge service registry bridge
type Bridge struct {
	sync.Mutex
	registry RegistryAdapter
	// services map[string]*Service
	store  *Storage
	config Config
}

// New returns a new registry bridge
func New(adapterURI string, config Config) (*Bridge, error) {
	uri, err := url.Parse(adapterURI)
	if err != nil {
		return nil, errors.New("bad adapter uri: " + adapterURI)
	}

	factory, found := AdapterFactories.Lookup(uri.Scheme)
	if !found {
		return nil, errors.New("unreconized adapter: " + adapterURI)
	}

	store, err := NewStore(config.ConfDir)
	if err != nil {
		return nil, errors.New("init local filesystem store: " + err.Error())
	}

	log.Infof("using %s adapter: %s", uri.Scheme, adapterURI)

	return &Bridge{
		config:   config,
		registry: factory.New(uri),
		// services: make(map[string]*Service),
		store: store,
	}, nil
}

// Ping testing backend is connect
func (b *Bridge) Ping() error {
	return b.registry.Ping()
}

// Add try to add service to backend
func (b *Bridge) Add(path string) {
	b.Lock()
	defer b.Unlock()

	b.add(path, false)
}

// Remove try to remove service to backend
func (b *Bridge) Remove(path string) {
	b.remove(path)
}

// Rename try to remove service to backend If rename path has already registered
func (b *Bridge) Rename(path string) {
	// validate service has already registered
	if _, ok := b.store.GetServiceID(path); !ok {
		return
	}
	b.remove(path)
}

// Update try to update service to backend
func (b *Bridge) Update(path string) {
	b.Remove(path)
	b.Add(path)
}

// Refresh refresh service registry ttl
func (b *Bridge) Refresh() {

}

// Sync sync service to backend
func (b *Bridge) Sync(quiet bool) {
	b.Lock()
	defer b.Unlock()

	paths, err := RecursiveFilesLookup(b.config.ConfDir, "*json")
	if err != nil && quiet {
		log.Errorf("recursive lookup confdir failed: %v", err)
		return
	} else if err != nil && !quiet {
		log.Fatal(err)
	}

	log.Infof("syncing services on %d files (include storage.json)", len(paths))

	registered := []string{}

	for _, path := range paths {
		if filepath.Base(path) == b.store.FileName {
			continue
		}
		service := b.newService(path)
		registered = append(registered, service.ID)

		if sid, ok := b.store.GetServiceID(path); !ok || sid != service.ID {
			b.add(path, quiet)
		} else {
			err := b.registry.Register(service)
			if err != nil {
				log.Errorf("sync register failed: %v %v", service, err)
			}
		}
	}

	if b.config.Cleanup {
		extServices, err := b.registry.Services()
		if err != nil {
			log.Errorf("cleanup failed: %v", err)
			return
		}

	Outer:
		for _, extService := range extServices {
			matches := serviceIDPattern.FindStringSubmatch(extService.ID)
			if len(matches) != 3 {
				// There's no way this was registered by us, so leave it
				continue
			}

			hostname := matches[1]
			if hostname != Hostname {
				// ignore because registered on a different host
				continue
			}
			sign := matches[2]

			for _, v := range b.store.Services() {
				if sign == v.SHA1 {
					continue Outer
				}
			}

			log.Infof("dangling: %s", extService.ID)
			err := b.registry.Deregister(extService)
			if err != nil {
				log.Errorf("deregister failed: %s %v", extService.ID, err)
				continue
			}
			log.Infof("%s removed", extService.ID)
		}
	}
}

func (b *Bridge) add(path string, quiet bool) {
	service := b.newService(path)
	if id, ok := b.store.GetServiceID(path); ok && service.ID == id {
		log.Warnf("ignored service registry request, it's already registered path: %s, service_id: %s", path, id)
		return
	}

	if service == nil {
		if !quiet {
			log.Warnf("new service with file %s failed, ignored", path)
		}
		return
	}
	err := b.registry.Register(service)
	if err != nil {
		log.Errorf("register service failed: %v", err)
		return
	}

	err = b.store.Add(path, service.ID)
	if err != nil {
		log.Errorf("register service succeed, but persistent in local storage failed: %v", err)
		return
	}

	log.Infof("added: %s %s", path, service.ID)
}

func (b *Bridge) remove(path string) {
	b.Lock()
	defer b.Unlock()

	if id, ok := b.store.GetServiceID(path); ok {
		log.Infof("removed: %s %s", path, id)
		svc := &Service{ID: id}
		b.registry.Deregister(svc)
		b.store.Remove(path)
	}

}

func (b *Bridge) newService(path string) *Service {
	service := new(Service)
	configBytes, err := ioutil.ReadFile(path)
	if err != nil {
		log.Errorf("read service definition config %s failed: %v", path, err)
		return nil
	}
	if err = json.Unmarshal(configBytes, service); err != nil {
		log.Errorf("parse service definition config %s failed: %v", path, err)
		return nil
	}

	hostname := Hostname
	service.ID = fmt.Sprintf("[%s]:%s:%d", hostname, b.signature(configBytes), service.Port)
	service.TTL = b.config.RefreshTTL

	return service
}

func (b *Bridge) signature(bytes []byte) string {
	bs := sha1.Sum(bytes)
	return fmt.Sprintf("%x", bs)
}

var Hostname string

func init() {
	Hostname, _ = os.Hostname()
}
