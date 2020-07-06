package bridge

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
)

// StorageName is the storage file name
const StorageName = "storage.json"

// ServiceMeta is represent service registered metadata
type ServiceMeta struct {
	ID   string `json:"service_id"`
	SHA1 string `json:"service_sha1"`
}

// Storage is a way to store
type Storage struct {
	sync.Mutex
	ConfDir  string                 `json:"-"`
	FileName string                 `json:"-"`
	Metadata map[string]ServiceMeta `json:"metadata"`
}

// NewStore returns an new storage
func NewStore(configdir string) (*Storage, error) {
	var storage Storage
	storage.ConfDir = configdir
	storage.FileName = StorageName

	configBytes, err := ioutil.ReadFile(filepath.Join(configdir, StorageName))
	if err != nil && os.IsNotExist(err) {
		storage.Metadata = make(map[string]ServiceMeta)
		return &storage, nil
	} else if err != nil {
		return nil, err
	}

	err = json.Unmarshal(configBytes, &storage)
	if err != nil {
		return nil, err
	}

	return &storage, nil
}

// Add add registered service to local storage
func (fs *Storage) Add(name string, serviceID string) error {
	fs.Lock()
	defer fs.Unlock()

	configBytes, err := ioutil.ReadFile(name)
	if err != nil {
		return err
	}

	meta := ServiceMeta{
		ID:   serviceID,
		SHA1: fmt.Sprintf("%x", sha1.Sum(configBytes)),
	}

	fs.Metadata[name] = meta
	return fs.flush()
}

// Remove remove registered service from local storage
func (fs *Storage) Remove(name string) error {
	fs.Lock()
	defer fs.Unlock()

	delete(fs.Metadata, name)

	return fs.flush()
}

// GetServiceID returns a service register id If exists
func (fs *Storage) GetServiceID(name string) (string, bool) {
	fs.Lock()
	defer fs.Unlock()

	meta, ok := fs.Metadata[name]

	if ok {
		return meta.ID, ok
	}

	return "", ok
}

// Services returns a list of registered service in local storage
func (fs *Storage) Services() []ServiceMeta {
	fs.Lock()
	defer fs.Unlock()

	results := make([]ServiceMeta, 0, len(fs.Metadata))
	for _, meta := range fs.Metadata {
		results = append(results, meta)
	}

	return results
}

func (fs *Storage) flush() error {
	configBytes, err := json.Marshal(fs)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filepath.Join(fs.ConfDir, StorageName), configBytes, os.ModePerm)
}
