package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	nested "github.com/antonfisher/nested-logrus-formatter"
	"github.com/feifeigood/registrator/bridge"
	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"

	_ "github.com/feifeigood/registrator/consul"
	_ "github.com/feifeigood/registrator/consulkv"
)

const app = "registrator"

// Build information.
var (
	Version string
	GitSHA  string
)

var printVersion = flag.Bool("version", false, "print version and exit.")
var confdir = flag.String("config-dir", "/etc/registrator", "service definition config dir, include config file must be end in the '.json'")
var refreshInterval = flag.Int("ttl-refresh", 0, "frequency with which services are resynchronized")
var refreshTTL = flag.Int("ttl", 0, "TTL for services (default is no expiry)")
var retryAttempts = flag.Int("retry-attempts", 0, "max retry attempts to establish a connection with the backend. Use -1 for infinite retries")
var retryInterval = flag.Int("retry-interval", 2000, "interval (in millisecond) between retry-attempts")
var resyncInterval = flag.Int("resync", 0, "frequency with which services are resynchronized")
var hostIP = flag.String("ip", "", "ip for ports mapped to the host")
var cleanup = flag.Bool("cleanup", false, "remove dangling services")

var log = logrus.WithField("component", "main")

func init() {
	logrus.SetFormatter(&nested.Formatter{
		TimestampFormat: time.RFC3339,
		HideKeys:        true,
		FieldsOrder:     []string{"component"},
	})
}

func failOnError(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s [options] <registry URI>\n\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()

	if *printVersion {
		fmt.Printf("%s Version: %s\n", app, Version)
		fmt.Printf("Git SHA: %s\n", GitSHA)
		fmt.Printf("Go Version: %s\n", runtime.Version())
		fmt.Printf("Go OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
		os.Exit(0)
	}

	if flag.NArg() != 1 {
		if flag.NArg() == 0 {
			fmt.Fprintf(os.Stderr, "Missing required argument for registry URI.\n\n")
		} else {
			fmt.Fprintln(os.Stderr, "Extra unparsed arguments:")
			fmt.Fprintln(os.Stderr, " ", strings.Join(flag.Args()[1:], " "))
			fmt.Fprintf(os.Stderr, "Options should come before the registry URI argument.\n\n")
		}
		flag.Usage()
		os.Exit(2)
	}

	if *hostIP != "" {
		log.Infof("using host IP to %s", *hostIP)
	}

	if (*refreshInterval > 0 && *refreshTTL == 0) || (*refreshInterval == 0 && *refreshTTL > 0) {
		failOnError(errors.New("-ttl and -ttl-refresh must be specified together or not at all"))
	} else if *refreshTTL > 0 && *refreshTTL <= *refreshInterval {
		failOnError(errors.New("-ttl must be grether than -ttl-refresh"))
	}

	if *retryInterval <= 0 {
		failOnError(errors.New("-retry-interval must be grether than 0"))
	}

	log.Infof("starting registrator %s", Version)

	b, err := bridge.New(flag.Arg(0), bridge.Config{
		HostIP:          *hostIP,
		RefreshInterval: *refreshInterval,
		RefreshTTL:      *refreshTTL,
		ConfDir:         *confdir,
		Cleanup:         *cleanup,
	})

	failOnError(err)

	attempt := 0

	for *retryAttempts == -1 || attempt <= *retryAttempts {
		log.Infof("connecting to backend (%v/%v)", attempt, *retryAttempts)
		err := b.Ping()
		if err == nil {
			break
		}

		if err != nil && attempt == *retryAttempts {
			failOnError(err)
		}
		time.Sleep(time.Duration(*retryInterval) * time.Millisecond)
		attempt++
	}

	// Start fsnotify
	watcher, err := fsnotify.NewWatcher()
	failOnError(err)
	defer watcher.Close()

	quit := make(chan os.Signal, 1)
	stop := make(chan struct{})
	wg := &sync.WaitGroup{}

	b.Sync(false)

	log.Infof("listening for fsnotify events ...")
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				// ignore invalid file, like vim .swap
				log.Debugf("received fsnotify event: %v", event)
				if filepath.Ext(event.Name) != ".json" || event.Name == bridge.StorageName {
					continue
				}

				switch event.Op {
				case fsnotify.Create:
					b.Add(event.Name)
				case fsnotify.Remove:
					b.Remove(event.Name)
				default:
					log.Debugf("received fsnotify event: %v, ignored", event)
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Errorf("fsnotify watcher failed: %v", err)
			}
		}
	}()

	err = watcher.Add(*confdir)
	failOnError(err)

	// Start the TTL refresh timer
	if *refreshInterval > 0 {
		wg.Add(1)
		ticker := time.NewTicker(time.Duration(*refreshInterval) * time.Second)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ticker.C:
					b.Refresh()
				case <-stop:
					ticker.Stop()
					return
				}
			}
		}()
	}

	// Start the resync timer if enabled
	if *resyncInterval > 0 {
		log.Infof("interval %v for resynchronized", time.Duration(*resyncInterval)*time.Second)
		wg.Add(1)
		resyncTicker := time.NewTicker(time.Duration(*resyncInterval) * time.Second)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-resyncTicker.C:
					b.Sync(false)
				case <-stop:
					resyncTicker.Stop()
					return
				}
			}
		}()
	}

	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit
	close(stop)

	wg.Wait()
}
