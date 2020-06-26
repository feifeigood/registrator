# Registrator

Service registry bridge using file-base model with Node.


## Usage
```code
Usage of ./registrator:
  ./registrator [options] <registry URI>

  -cleanup
    	remove dangling services
  -config-dir string
    	service definition config dir, include config file must be end in the '.json' (default "/etc/registrator")
  -ip string
    	ip for ports mapped to the host
  -resync int
    	frequency with which services are resynchronized (default 600)
  -retry-attempts int
    	max retry attempts to establish a connection with the backend. Use -1 for infinite retries
  -retry-interval int
    	interval (in millisecond) between retry-attempts (default 2000)
  -ttl int
    	TTL for services (default is no expiry)
  -ttl-refresh int
    	frequency with which services are resynchronized
  -version
    	print version and exit.
```

