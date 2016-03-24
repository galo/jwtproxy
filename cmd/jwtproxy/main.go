// Copyright 2015 CoreOS, Inc
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

package main

import (
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	log "github.com/Sirupsen/logrus"

	"github.com/coreos-inc/jwtproxy/config"
	"github.com/coreos-inc/jwtproxy/jwt"
	"github.com/coreos-inc/jwtproxy/proxy"

	_ "github.com/coreos-inc/jwtproxy/jwt/keyserver/preshared"
	_ "github.com/coreos-inc/jwtproxy/jwt/noncestorage/local"
	_ "github.com/coreos-inc/jwtproxy/jwt/privatekey/preshared"
)

func main() {
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	flagConfigPath := flag.String("config", "", "Load configuration from the specified yaml file.")
	flagLogLevel := flag.String("log-level", "info", "Define the logging level.")
	flag.Parse()

	// Load configuration.
	config, err := config.Load(*flagConfigPath)
	if err != nil {
		flag.Usage()
		log.Fatalf("Failed to load configuration: %s", err)
	}

	// Initialize logging system.
	level, err := log.ParseLevel(*flagLogLevel)
	if err != nil {
		log.Fatalf("Failed to parse the log level: %s", err)
	}
	log.SetLevel(level)

	// Create JWT proxy handlers.
	fwp, err := jwt.NewJWTSignerHandler(config.SignerProxy.Signer)
	if err != nil {
		log.Fatalf("Failed to create JWT signer: %s", err)
	}
	rvp, err := jwt.NewJWTVerifierHandler(config.VerifierProxy.Verifier)
	if err != nil {
		log.Fatalf("Failed to create JWT verifier: %s", err)
	}

	// Create forward and reverse proxies.
	forwardProxy, err := proxy.NewProxy(fwp, config.SignerProxy.CAKeyFile, config.SignerProxy.CACrtFile)
	if err != nil {
		log.Fatalf("Failed to create forward proxy: %s", err)
	}

	reverseProxy, err := proxy.NewReverseProxy(rvp)
	if err != nil {
		log.Fatalf("Failed to create reverse proxy: %s", err)
	}

	// Start proxies.
	go func() {
		log.Infof("Starting forward proxy (Listening on '%s')", config.SignerProxy.ListenAddr)
		log.Fatal(http.ListenAndServe(config.SignerProxy.ListenAddr, forwardProxy))
	}()

	go func() {
		if config.VerifierProxy.CrtFile != "" && config.VerifierProxy.KeyFile != "" {
			log.Infof("Starting reverse proxy (Listening on '%s', TLS Enabled)", config.VerifierProxy.ListenAddr)
			log.Fatal(http.ListenAndServeTLS(config.VerifierProxy.ListenAddr, config.VerifierProxy.CrtFile, config.VerifierProxy.KeyFile, reverseProxy))
		} else {
			log.Infof("Starting reverse proxy (Listening on '%s', TLS Disabled)", config.VerifierProxy.ListenAddr)
			go log.Fatal(http.ListenAndServe(config.VerifierProxy.ListenAddr, reverseProxy))
		}
	}()

	waitForSignals(syscall.SIGINT, syscall.SIGTERM)
	// TODO: Graceful stop.
}

func waitForSignals(signals ...os.Signal) {
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, signals...)
	<-interrupts
}
