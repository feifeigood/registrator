package bridge

import (
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
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
			signature := matches[2]
			for i := range b.services {
				if signature == serviceIDPattern.FindStringSubmatch(i)[2] {
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
	if service == nil {
		if !quiet {
			log.Warnf("register %s had some error occured, ignored", path)
		}
		return
	}
	err := b.registry.Register(service)
	if err != nil {
		log.Errorf("register %s failed: %v", path, err)
		return
	}

	b.services[service.ID] = service
	log.Infof("added: %s %s", path, service.ID)
}

func (b *Bridge) remove(path string) {
	b.Lock()
	defer b.Unlock()

	singature := b.signature(path)
	for i, svc := range b.services {
		matches := serviceIDPattern.FindStringSubmatch(i)
		if len(matches) != 3 {
			// There's no way this was registered by us, so leave it
			continue
		}

		if singature != matches[2] {
			continue
		}

		log.Infof("removed: %s %s", path, i)

		b.registry.Deregister(svc)
		delete(b.services, i)
	}
}

func (b *Bridge) newService(path string) *Service {
	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		log.Errorf("read %s: %v", path, err)
		return nil
	}
	svc := new(Service)
	if err = json.Unmarshal(bytes, svc); err != nil {
		log.Errorf("parse %s: %v", path, err)
		return nil
	}

	hostname := Hostname
	svc.ID = fmt.Sprintf("[%s]:%s:%d", hostname, b.signature(path), svc.Port)
	svc.TTL = b.config.RefreshTTL

	return svc
}

func (b *Bridge) signature(path string) string {
	bs := sha1.Sum([]byte(path))
	return fmt.Sprintf("%x", bs)
}

var Hostname string

func init() {
	Hostname, _ = os.Hostname()
}
