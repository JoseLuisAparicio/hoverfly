// Copyright 2015 SpectoLabs. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// hoverfly is an HTTP/s proxy configurable via flags/environment variables/admin HTTP API
//
// this proxy can be dynamically configured through HTTP calls when it's running, to change modes,
// export and import requests.

package main

import (
	"bufio"
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/SpectoLabs/goproxy"
	hv "github.com/SpectoLabs/hoverfly/core"
	"github.com/SpectoLabs/hoverfly/core/authentication/backends"
	"github.com/SpectoLabs/hoverfly/core/cache"
	hvc "github.com/SpectoLabs/hoverfly/core/certs"
	"github.com/SpectoLabs/hoverfly/core/matching"
	"github.com/SpectoLabs/hoverfly/core/modes"
)

type arrayFlags []string

func (i *arrayFlags) String() string {
	return "my string representation"
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

var importFlags arrayFlags
var destinationFlags arrayFlags

const boltBackend = "boltdb"
const inmemoryBackend = "memory"

var (
	version     = flag.Bool("version", false, "get the version of hoverfly")
	verbose     = flag.Bool("v", false, "should every proxy request be logged to stdout")
	capture     = flag.Bool("capture", false, "start Hoverfly in capture mode - transparently intercepts and saves requests/response")
	synthesize  = flag.Bool("synthesize", false, "start Hoverfly in synthesize mode (middleware is required)")
	modify      = flag.Bool("modify", false, "start Hoverfly in modify mode - applies middleware (required) to both outgoing and incomming HTTP traffic")
	middleware  = flag.String("middleware", "", "should proxy use middleware")
	proxyPort   = flag.String("pp", "", "proxy port - run proxy on another port (i.e. '-pp 9999' to run proxy on port 9999)")
	adminPort   = flag.String("ap", "", "admin port - run admin interface on another port (i.e. '-ap 1234' to run admin UI on port 1234)")
	metrics     = flag.Bool("metrics", false, "supply -metrics flag to enable metrics logging to stdout")
	dev         = flag.Bool("dev", false, "supply -dev flag to serve directly from ./static/dist instead from statik binary")
	destination = flag.String("destination", ".", "destination URI to catch")
	webserver   = flag.Bool("webserver", false, "start Hoverfly in webserver mode (simulate mode)")

	addNew          = flag.Bool("add", false, "add new user '-add -username hfadmin -password hfpass'")
	addUser         = flag.String("username", "", "username for new user")
	addPassword     = flag.String("password", "", "password for new user")
	addPasswordHash = flag.String("password-hash", "", "password hash for new user instead of password")
	isAdmin         = flag.Bool("admin", true, "supply '-admin false' to make this non admin user (defaults to 'true') ")
	authEnabled     = flag.Bool("auth", false, "enable authentication, currently it is disabled by default")

	proxyAuthorizationHeader = flag.String("proxy-auth", "proxy-auth", "Switch the Proxy-Authorization header from proxy-auth `Proxy-Authorization` to header-auth `X-HOVERFLY-AUTHORIZATION`. Switching to header-auth will auto enable -https-only")

	generateCA = flag.Bool("generate-ca-cert", false, "generate CA certificate and private key for MITM")
	certName   = flag.String("cert-name", "hoverfly.proxy", "cert name")
	certOrg    = flag.String("cert-org", "Hoverfly Authority", "organisation name for new cert")
	cert       = flag.String("cert", "", "CA certificate used to sign MITM certificates")
	key        = flag.String("key", "", "private key of the CA used to sign MITM certificates")

	tlsVerification = flag.Bool("tls-verification", true, "turn on/off tls verification for outgoing requests (will not try to verify certificates) - defaults to true")

	upstreamProxy = flag.String("upstream-proxy", "", "specify an upstream proxy for hoverfly to route traffic through")
	httpsOnly     = flag.Bool("https-only", false, "allow only secure secure requests to be proxied by hoverfly")

	databasePath = flag.String("db-path", "", "database location - supply it to provide specific database location (will be created there if it doesn't exist)")
	database     = flag.String("db", inmemoryBackend, "Persistance storage to use - 'boltdb' or 'memory' which will not write anything to disk")
	disableCache = flag.Bool("disable-cache", false, "Disable the cache that sits infront of matching")
)

var CA_CERT = []byte(`-----BEGIN CERTIFICATE-----
MIIDbTCCAlWgAwIBAgIVAPAvY6MQi4KmJYmPDmnE29y6njABMA0GCSqGSIb3DQEB
CwUAMDYxGzAZBgNVBAoTEkhvdmVyZmx5IEF1dGhvcml0eTEXMBUGA1UEAxMOaG92
ZXJmbHkucHJveHkwHhcNMTIwMzI1MTM0NjI3WhcNMjIwMzIzMTM0NjI3WjA2MRsw
GQYDVQQKExJIb3ZlcmZseSBBdXRob3JpdHkxFzAVBgNVBAMTDmhvdmVyZmx5LnBy
b3h5MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAsw2DShgDHkAugLb2
efVq5XYPIiiJa1Dj+DPxQEuQDtQYAJPgGm7aCm7YLke0Gm6p2ZJBtLmEEwhwRw50
f6oeWdd21G2RvnzWLOM8QLehUDtQUxO1pMO4prrP3WmTm/UQr0n50BCC/W/omJIZ
tdmTN5Z1kHaiYcLeOiHVzzAoVlj45vBS2Tm7guAxWMNAnvzGAif0F0LsTCLIzQBg
eZ6CQeOe0neS1pCGr4NrxuX6pDu/T/YnS+x6P+g0jUOnlwtQsGPjh1Vw0hhZJe6Z
/YdnZrIufRaAEufbq8dk/ELZVT4Mi6Gp5uy0gycnWhf1mPhsKpbEOhv1r8tEYQrn
5u4cHwIDAQABo3IwcDAOBgNVHQ8BAf8EBAMCAqQwEwYDVR0lBAwwCgYIKwYBBQUH
AwEwDwYDVR0TAQH/BAUwAwEB/zAdBgNVHQ4EFgQUxTebj9Kv16fuWngIO4zfjddv
7fUwGQYDVR0RBBIwEIIOaG92ZXJmbHkucHJveHkwDQYJKoZIhvcNAQELBQADggEB
AKOlihA/DIAc7soiPb8s5eLvY/YTqASgzy3S1oaqEsEFAnNPOu53ePNid6bmKDvD
hc0E+sphPpcuWyzoSp4Nz5TFl7LTtIzU49mR+/Gn3tDucGLutS0PbdFen7swKTMO
/HwXy+Bm2a9g4ewgJbfIf4MhgrdX6M4gJqMVL7q/NKeppHlQ4pBYFc+HjrF4V98V
x/mHLc65qOh7iKPBVY7lSYipnMxRu5N7Q88eaqfaVh44xsIYO83N9Pn1uE9GOUu7
Eb2Tc6UidvXZMWAfkkrGVyrJGWE4wTAIT/Dz1AKDPuTp16SyljaOZ2YahmFXMp3C
Fj+GKkpM2WS40fUI9z40WGI=
-----END CERTIFICATE-----`)

var CA_KEY = []byte(`-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEAsw2DShgDHkAugLb2efVq5XYPIiiJa1Dj+DPxQEuQDtQYAJPg
Gm7aCm7YLke0Gm6p2ZJBtLmEEwhwRw50f6oeWdd21G2RvnzWLOM8QLehUDtQUxO1
pMO4prrP3WmTm/UQr0n50BCC/W/omJIZtdmTN5Z1kHaiYcLeOiHVzzAoVlj45vBS
2Tm7guAxWMNAnvzGAif0F0LsTCLIzQBgeZ6CQeOe0neS1pCGr4NrxuX6pDu/T/Yn
S+x6P+g0jUOnlwtQsGPjh1Vw0hhZJe6Z/YdnZrIufRaAEufbq8dk/ELZVT4Mi6Gp
5uy0gycnWhf1mPhsKpbEOhv1r8tEYQrn5u4cHwIDAQABAoIBAQCmeAK/aXHEt0FE
9FZV70FSUyAgzvVsbAl3YruC3n3x+2jRaKqriLJ5jrK43HtrM8YAfYVPREex9l+F
AMB5TS3os3VMbQ5avu/VTfNf7BozYOH+S03PART1FqxZm2XcUs0PW8TBmAhhHqFu
8C6tLrs7rExjYpj4MVexTnHdrlViaMiMISXAEgiX5xJ8MNRgfWIhNYUssqXfOM95
wLF6Gq95Ma+4LSl/lXrg2Z28PbCWJLF4GzX4pX2EikGuSe9sf+Wq8NAY3XDwo9fT
I46WE13bZGQ/7ggkSrs3r0qrNtp+pO5XfFwV/UivQ7qaxHXKdeoWkFoTjxyblEa0
zzrpb5+xAoGBAOiwF42SxIeRCC65dNrHkGaYQlK/z11ly7ZKR9GRNr6+oAiq/Lqz
11NGQnZ00d2k7W3tk4oIrkT+bLO45LgxBCeMp97PLokY310c0y0Xjs0BL1RHczu6
8zuhGie/cda/5DH3vkQ6IlBXrh4gSpts865GbyXgLkrjsmgjbRDCdzE3AoGBAMT9
zfxlzCqa53LzqGyzf16CAr6t+Ry85NgKYbEZq87w9rzc68btbpoKBneUWuNIR7gR
tbksdNsc8F/s8D+ftWqHHkhHyALxzPEQy1I8wLPpwGFZPFSfeCmq6iJGSvNgTSgy
uv5PEAPpaDbBTKZ848U++eZOXGaIcx18KgcncgBZAoGBAJutjOSMaG6nCwlvzQ2+
7Q6nGeCRMiSzwZqBkhFVDYKKuTlzZMlpH0w4uqjUOcEH4k5k4Aw/CJFig8muj1/o
c3YedgXtKZ5SBMcgTO1jUIg6HbdOYnt49dlUTNKBFKHwGrWPoj21g1Wrg/Pl+OSJ
/XMA7sYxeedi9e8UnJjU8rf7AoGAPNTbnUuaRrXbL0ZLBnZPqNGhI1z6BoPWb1iV
Xmk9AwSqTRwzuxRrCSp7YMXxYypY62Ccq3gtBdTj7dtvPVaGYUUkdtGj1DTzQqYb
A2Q7ZdOTUvyJguBT7RoYf0kRsCJW8UjpMcscePjE89OxZeA/PhP6e8JLCmaslbhY
CimGLNECgYAGSyGzGL5eccyayX5e0uGCxRMqjYym8diaCftp+qKXUZw2z7IL8Tu6
AGCtSd0PcMk6IUmaG5mGWHJRb2mvi92Rhx1JUFfdc07FbHnNQBZfCj/SP26XROqp
nIoRtbUjdBkzPrwfSh22POoCdDUKlRUcwR0Wq7lrpQchSU1Xtz6Jkw==
-----END RSA PRIVATE KEY-----`)

func init() {
	// overriding default goproxy certificate
	tlsc, err := tls.X509KeyPair(CA_CERT, CA_KEY)
	if err != nil {
		log.Fatalf("Failed to load certifiate and key pair, got error: %s", err.Error())
	}
	goproxy.GoproxyCa = tlsc
}

func main() {
	hoverfly := hv.NewHoverfly()
	log.SetFormatter(&log.JSONFormatter{})
	flag.Var(&importFlags, "import", "import from file or from URL (i.e. '-import my_service.json' or '-import http://mypage.com/service_x.json'")
	flag.Var(&destinationFlags, "dest", "specify which hosts to process (i.e. '-dest fooservice.org -dest barservice.org -dest catservice.org') - other hosts will be ignored will passthrough'")
	flag.Parse()

	if *version {
		fmt.Println(hv.NewHoverfly().GetVersion())
		os.Exit(0)
	}

	// getting settings
	cfg := hv.InitSettings()

	if *verbose {
		// Only log the warning severity or above.
		log.SetLevel(log.DebugLevel)
	}
	cfg.Verbose = *verbose

	if *dev {
		// making text pretty
		log.SetFormatter(&log.TextFormatter{})
	}

	if *generateCA {
		tlsc, err := hvc.GenerateAndSave(*certName, *certOrg, 5*365*24*time.Hour)
		if err != nil {
			log.WithFields(log.Fields{
				"error": err.Error(),
			}).Fatal("failed to generate certificate.")
		}
		goproxy.GoproxyCa = *tlsc

	} else if *cert != "" && *key != "" {
		tlsc, err := tls.LoadX509KeyPair(*cert, *key)
		if err != nil {
			log.Fatalf("Failed to load certifiate and key pair, got error: %s", err.Error())
		}

		log.WithFields(log.Fields{
			"certificate": *cert,
			"key":         *key,
		}).Info("Default keys have been overwritten")

		goproxy.GoproxyCa = tlsc

	}

	// overriding environment variables (proxy and admin ports)
	if *proxyPort != "" {
		cfg.ProxyPort = *proxyPort
	}
	if *adminPort != "" {
		cfg.AdminPort = *adminPort
	}

	// overriding environment variable (external proxy)
	if *upstreamProxy != "" {
		cfg.SetUpstreamProxy(*upstreamProxy)
	}

	cfg.HttpsOnly = *httpsOnly

	// development settings
	cfg.Development = *dev

	// overriding default middleware setting
	newMiddleware, err := hv.ConvertToNewMiddleware(*middleware)
	if err != nil {
		log.Error(err.Error())
	}
	cfg.Middleware = *newMiddleware

	mode := getInitialMode(cfg)

	// setting mode
	cfg.SetMode(mode)

	// disabling authentication if no-auth for auth disabled env variable
	if *authEnabled {
		cfg.AuthEnabled = true
	}

	// disabling tls verification if flag or env variable is set to 'false' (defaults to true)
	if !cfg.TLSVerification || !*tlsVerification {
		cfg.TLSVerification = false
		log.Info("tls certificate verification is now turned off!")
	}

	if len(destinationFlags) > 0 {
		cfg.Destination = strings.Join(destinationFlags[:], "|")

	} else {
		//  setting destination regexp
		cfg.Destination = *destination
	}

	var requestCache cache.Cache
	var metadataCache cache.Cache
	var tokenCache cache.Cache
	var userCache cache.Cache

	if *databasePath != "" {
		cfg.DatabasePath = *databasePath
	}

	if *database == boltBackend {
		log.Info("Creating bolt db backend...")
		db := cache.GetDB(cfg.DatabasePath)
		defer db.Close()
		requestCache = cache.NewBoltDBCache(db, []byte("requestsBucket"))
		metadataCache = cache.NewBoltDBCache(db, []byte("metadataBucket"))
		tokenCache = cache.NewBoltDBCache(db, []byte(backends.TokenBucketName))
		userCache = cache.NewBoltDBCache(db, []byte(backends.UserBucketName))
	} else if *database == inmemoryBackend {
		log.Info("Creating in memory map backend...")
		log.Warn("Turning off authentication...")

		requestCache = cache.NewInMemoryCache()
		metadataCache = cache.NewInMemoryCache()
		tokenCache = cache.NewInMemoryCache()
		userCache = cache.NewInMemoryCache()
	} else {
		log.Fatalf("unknown database type chosen: %s", *database)
	}
	cfg.DisableCache = *disableCache
	if cfg.DisableCache {
		requestCache = nil
	}

	if *proxyAuthorizationHeader == "header-auth" {
		log.Warnf("Proxy authentication will use `X-HOVERFLY-AUTHORIZATION` instead of `Proxy-Authorization`")
		cfg.ProxyAuthorizationHeader = "X-HOVERFLY-AUTHORIZATION"
		log.Warnf("Setting Hoverfly to only proxy HTTPS requests")
		cfg.HttpsOnly = true
	}

	authBackend := backends.NewCacheBasedAuthBackend(tokenCache, userCache)

	hoverfly.Cfg = cfg
	hoverfly.CacheMatcher = matching.CacheMatcher{
		RequestCache: requestCache,
		Webserver:    cfg.Webserver,
	}
	hoverfly.MetadataCache = metadataCache
	hoverfly.Authentication = authBackend
	hoverfly.HTTP = hv.GetDefaultHoverflyHTTPClient(hoverfly.Cfg.TLSVerification, hoverfly.Cfg.UpstreamProxy)

	// if add new user supplied - adding it to database
	if *addNew || *authEnabled {
		var err error
		if *addPasswordHash != "" {
			err = hoverfly.Authentication.AddUserHashedPassword(*addUser, *addPasswordHash, *isAdmin)
		} else {
			err = hoverfly.Authentication.AddUser(*addUser, *addPassword, *isAdmin)
		}
		if err != nil {
			log.WithFields(log.Fields{
				"error":    err.Error(),
				"username": *addUser,
			}).Fatal("failed to add new user")
		} else {
			log.WithFields(log.Fields{
				"username": *addUser,
			}).Info("user added successfuly")
		}
		cfg.AuthEnabled = true
	}
	if cfg.AuthEnabled {
		if os.Getenv(hv.HoverflyAdminUsernameEV) != "" && os.Getenv(hv.HoverflyAdminPasswordEV) != "" {
			hoverfly.Authentication.AddUser(
				os.Getenv(hv.HoverflyAdminUsernameEV),
				os.Getenv(hv.HoverflyAdminPasswordEV),
				true)
		}

		// checking if there are any users
		users, err := hoverfly.Authentication.GetAllUsers()
		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
			}).Fatal("got error while trying to get all users")
		}
		if len(users) < 1 {
			createSuperUser(hoverfly)
		}
	}

	// importing records if environment variable is set
	ev := os.Getenv(hv.HoverflyImportRecordsEV)
	if ev != "" {
		err := hoverfly.Import(ev)
		if err != nil {
			log.WithFields(log.Fields{
				"error":  err.Error(),
				"import": ev,
			}).Fatal("Environment variable for importing was set but failed to import this resource")
		} else {
			err = hoverfly.MetadataCache.Set([]byte("import_from_env_variable"), []byte(ev))
		}
	}

	// importing stuff
	if len(importFlags) > 0 {
		for i, v := range importFlags {
			if v != "" {
				log.WithFields(log.Fields{
					"import": v,
				}).Debug("Importing given resource")
				err := hoverfly.Import(v)
				if err != nil {
					log.WithFields(log.Fields{
						"error":  err.Error(),
						"import": v,
					}).Fatal("Failed to import given resource")
				} else {
					err = hoverfly.MetadataCache.Set([]byte(fmt.Sprintf("import_%d", i+1)), []byte(v))
				}
			}
		}
	}

	// start metrics registry flush
	if *metrics {
		hoverfly.Counter.Init()
	}

	cfg.Webserver = *webserver

	err = hoverfly.StartProxy()
	if err != nil {
		log.WithFields(log.Fields{
			"error": err.Error(),
		}).Fatal("failed to start proxy...")
	}

	// starting admin interface, this is blocking
	adminApi := hv.AdminApi{}
	adminApi.StartAdminInterface(hoverfly)
}

func createSuperUser(h *hv.Hoverfly) {
	reader := bufio.NewReader(os.Stdin)
	// Prompt and read
	fmt.Println("No users found in the database, please create initial user.")
	fmt.Print("Enter username (default hf): ")
	username, err := reader.ReadString('\n')
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Fatal("error while getting username input")
	}
	fmt.Print("Enter password (default hf): ")
	password, err := reader.ReadString('\n')
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Fatal("error while getting password input")
	}
	// Trim whitespace and use defaults if nothing entered
	username = strings.TrimSpace(username)
	if username == "" {
		username = "hf"
	}
	password = strings.TrimSpace(password)
	if password == "" {
		password = "hf"
	}
	err = h.Authentication.AddUser(username, password, true)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Fatal("failed to create user.")
	} else {
		log.Infof("User: '%s' created.\n", username)
	}
}

func getInitialMode(cfg *hv.Configuration) string {
	if *webserver {
		return modes.Simulate
	}

	if *capture {
		// checking whether user supplied other modes
		if *synthesize == true || *modify == true {
			log.Fatal("Two or more modes supplied, check your flags")
		}

		return modes.Capture

	} else if *synthesize {

		if !cfg.Middleware.IsSet() {
			log.Fatal("Synthesize mode chosen although middleware not supplied")
		}

		if *capture == true || *modify == true {
			log.Fatal("Two or more modes supplied, check your flags")
		}

		return modes.Synthesize

	} else if *modify {
		if !cfg.Middleware.IsSet() {
			log.Fatal("Modify mode chosen although middleware not supplied")
		}

		if *capture == true || *synthesize == true {
			log.Fatal("Two or more modes supplied, check your flags")
		}

		return modes.Modify
	}

	return modes.Simulate
}
