package bridge

import (
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

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

	log.Infof("%v", AdapterFactories.All())
	factory, found := AdapterFactories.Lookup(uri.Scheme)
	if !found {
		return nil, errors.New("unreconized adapter: " + adapterURI)
	}

	log.Infof("Using %s adapter: %s", uri.Scheme, adapterURI)
	return &Bridge{
		config:   config,
		registry: factory.New(uri),
		services: make(map[string]*Service),
	}, nil
}

func (b *Bridge) Ping() error {
	return b.registry.Ping()
}

func (b *Bridge) Add(path string) {
	b.Lock()
	defer b.Unlock()
	b.add(path, false)
}

func (b *Bridge) Remove(path string) {
	b.remove(path)
}

func (b *Bridge) Refresh() {

}

func (b *Bridge) Sync(quiet bool) {

	//

}

func (b *Bridge) add(path string, quiet bool) {
	service := b.newService(path)
	if service == nil {
		if !quiet {
			log.Warnf("Register %s had some error occured, ignored", path)
		}
		return
	}
	err := b.registry.Register(service)
	if err != nil {
		log.Errorf("Register %s failed: %v", path, err)
		return
	}

	b.services[service.ID] = service
	log.Infof("Added: %s %s", path, service.ID)
}

func (b *Bridge) remove(path string) {
	b.Lock()
	defer b.Unlock()

	prefix := b.config.HostIP + "-" + pathsha1(path)

	serviceID := ""

	for id := range b.services {
		if strings.HasPrefix(id, prefix) {
			serviceID = id
			break
		}
	}

	if serviceID != "" {
		b.registry.Deregister(b.services[serviceID])
		delete(b.services, serviceID)
	}
}

func (b *Bridge) newService(path string) *Service {
	// new service from definition config
	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		log.Errorf("Read %s: %v", path, err)
		return nil
	}
	svc := new(Service)
	if err = json.Unmarshal(bytes, svc); err != nil {
		log.Errorf("Parse %s: %v", path, err)
		return nil
	}

	svc.ID = b.config.HostIP + "-" + pathsha1(path) + "-" + fmt.Sprintf("%d", svc.Port)
	svc.TTL = b.config.RefreshTTL

	return svc
}

func pathsha1(path string) string {
	h := sha1.New()
	h.Write([]byte(path))
	bs := h.Sum(nil)
	return fmt.Sprintf("%x", bs)
}
