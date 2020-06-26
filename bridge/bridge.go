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
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

var serviceIDPattern = regexp.MustCompile(`^\[(.+?)\]:([a-zA-Z0-9][a-zA-Z0-9_.-]+):[0-9]+$`)

var log = logrus.WithField("component", "bridge")

// Bridge service registry bridge
type Bridge struct {
	sync.Mutex
	registry RegistryAdapter
	services map[string]*Service
	config   Config
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

	log.Infof("using %s adapter: %s", uri.Scheme, adapterURI)
	return &Bridge{
		config:   config,
		registry: factory.New(uri),
		services: make(map[string]*Service),
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

	log.Infof("syncing services on %d files", len(paths))

	registered := []string{}

	for _, path := range paths {
		service := b.newService(path)
		registered = append(registered, service.ID)
		if _, ok := b.services[service.ID]; !ok {
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
			rid := matches[2]
			for i := range b.services {
				if rid == serviceIDPattern.FindStringSubmatch(i)[2] {
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

	if b.hasRID(path) {
		// path already register, igonred
		b.services[service.ID] = service
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

	// rename service file
	rid := serviceIDPattern.FindStringSubmatch(service.ID)[2]
	newpath := fmt.Sprintf("%s-rid%s.json", strings.TrimSuffix(path, filepath.Ext(path)), rid)
	os.Rename(path, newpath)

	b.services[service.ID] = service
	log.Infof("added: %s %s", newpath, service.ID)
}

var rIDPattern = regexp.MustCompile(`^.+-rid(.+?)$`)

func (b *Bridge) hasRID(path string) bool {
	fn := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	matches := rIDPattern.FindStringSubmatch(fn)
	return len(matches) == 2
}

func (b *Bridge) remove(path string) {
	b.Lock()
	defer b.Unlock()

	if !b.hasRID(path) {
		return
	}

	rid := rIDPattern.FindStringSubmatch(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))[1]

	for id, svc := range b.services {
		matches := serviceIDPattern.FindStringSubmatch(id)
		if len(matches) != 3 {
			// There's no way this was registered by us, so leave it
			continue
		}

		if rid != matches[2] {
			continue
		}

		log.Infof("removed: %s %s", path, id)

		b.registry.Deregister(svc)
		delete(b.services, id)
	}
}

func (b *Bridge) newService(path string) *Service {
	service := new(Service)
	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		log.Errorf("read service definition config %s failed: %v", path, err)
		return nil
	}
	if err = json.Unmarshal(bytes, service); err != nil {
		log.Errorf("parse service definition config %s failed: %v", path, err)
		return nil
	}

	rid := GenerateRandomID()
	if b.hasRID(path) {
		rid = rIDPattern.FindStringSubmatch(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))[1]
	}

	hostname := Hostname
	service.ID = fmt.Sprintf("[%s]:%s:%d", hostname, rid, service.Port)
	service.TTL = b.config.RefreshTTL

	return service
}

func (b *Bridge) signature(path string) string {
	bs := sha1.Sum([]byte(path))
	return fmt.Sprintf("%x", bs)
}

var Hostname string

func init() {
	Hostname, _ = os.Hostname()
}
