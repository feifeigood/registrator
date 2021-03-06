package bridge

import (
	"net/url"
	"reflect"
	"sync"
)

// AdapterFactory adapter registry factory
type AdapterFactory interface {
	New(uri *url.URL) RegistryAdapter
}

// RegistryAdapter adapter for service registry backend
type RegistryAdapter interface {
	Ping() error
	Register(service *Service) error
	Deregister(service *Service) error
	Refresh(service *Service) error
	Services() ([]*Service, error)
}

// Config represent registry adapter config
type Config struct {
	HostIP          string
	RefreshTTL      int
	RefreshInterval int
	ConfDir         string
	Cleanup         bool
}

// Service registry service definition structure
type Service struct {
	ID    string
	Name  string            `json:"name"`
	Port  int               `json:"port"`
	IP    string            `json:"address"`
	Tags  []string          `json:"tags"`
	Attrs map[string]string `json:"attrs"`
	TTL   int
}

var registry = struct {
	sync.Mutex
	extpoints map[string]*extensionPoint
}{
	extpoints: make(map[string]*extensionPoint),
}

type extensionPoint struct {
	sync.Mutex
	iface      reflect.Type
	components map[string]interface{}
}

func newExtensionPoint(iface interface{}) *extensionPoint {
	ep := &extensionPoint{
		iface:      reflect.TypeOf(iface).Elem(),
		components: make(map[string]interface{}),
	}
	registry.Lock()
	defer registry.Unlock()
	registry.extpoints[ep.iface.Name()] = ep
	return ep
}

func (ep *extensionPoint) lookup(name string) (ext interface{}, ok bool) {
	ep.Lock()
	defer ep.Unlock()
	ext, ok = ep.components[name]
	return
}

func (ep *extensionPoint) all() map[string]interface{} {
	ep.Lock()
	defer ep.Unlock()
	all := make(map[string]interface{})
	for k, v := range ep.components {
		all[k] = v
	}
	return all
}

func (ep *extensionPoint) register(component interface{}, name string) bool {
	ep.Lock()
	defer ep.Unlock()
	if name == "" {
		name = reflect.TypeOf(component).Elem().Name()
	}
	_, exists := ep.components[name]
	if exists {
		return false
	}
	ep.components[name] = component
	return true
}

func (ep *extensionPoint) unregister(name string) bool {
	ep.Lock()
	defer ep.Unlock()
	_, exists := ep.components[name]
	if !exists {
		return false
	}
	delete(ep.components, name)
	return true
}

func implements(component interface{}) []string {
	var ifaces []string
	for name, ep := range registry.extpoints {
		if reflect.TypeOf(component).Implements(ep.iface) {
			ifaces = append(ifaces, name)
		}
	}
	return ifaces
}

func Register(component interface{}, name string) []string {
	registry.Lock()
	defer registry.Unlock()
	var ifaces []string
	for _, iface := range implements(component) {
		if ok := registry.extpoints[iface].register(component, name); ok {
			ifaces = append(ifaces, iface)
		}
	}
	return ifaces
}

func Unregister(name string) []string {
	registry.Lock()
	defer registry.Unlock()
	var ifaces []string
	for iface, extpoint := range registry.extpoints {
		if ok := extpoint.unregister(name); ok {
			ifaces = append(ifaces, iface)
		}
	}
	return ifaces
}

// AdapterFactory

var AdapterFactories = &adapterFactoryExt{
	newExtensionPoint(new(AdapterFactory)),
}

type adapterFactoryExt struct {
	*extensionPoint
}

func (ep *adapterFactoryExt) Unregister(name string) bool {
	return ep.unregister(name)
}

func (ep *adapterFactoryExt) Register(component AdapterFactory, name string) bool {
	return ep.register(component, name)
}

func (ep *adapterFactoryExt) Lookup(name string) (AdapterFactory, bool) {
	ext, ok := ep.lookup(name)
	if !ok {
		return nil, ok
	}
	return ext.(AdapterFactory), ok
}

func (ep *adapterFactoryExt) All() map[string]AdapterFactory {
	all := make(map[string]AdapterFactory)
	for k, v := range ep.all() {
		all[k] = v.(AdapterFactory)
	}
	return all
}
