package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/runconduit/conduit/controller/k8s"
	"github.com/runconduit/conduit/controller/tap"
	"github.com/runconduit/conduit/pkg/prometheus"
	"github.com/runconduit/conduit/pkg/version"
	log "github.com/sirupsen/logrus"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8088", "address to serve on")
	metricsAddr := flag.String("metrics-addr", ":9998", "address to serve scrapable metrics on")
	kubeConfigPath := flag.String("kubeconfig", "", "path to kube config")
	tapPort := flag.Uint("tap-port", 4190, "proxy tap port to connect to")
	logLevel := flag.String("log-level", log.InfoLevel.String(), "log level, must be one of: panic, fatal, error, warn, info, debug")
	printVersion := version.VersionFlag()
	flag.Parse()

	// set global log level
	level, err := log.ParseLevel(*logLevel)
	if err != nil {
		log.Fatalf("invalid log-level: %s", *logLevel)
	}
	log.SetLevel(level)

	version.MaybePrintVersionAndExit(*printVersion)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	clientSet, err := k8s.NewClientSet(*kubeConfigPath)
	if err != nil {
		log.Fatalf("failed to create Kubernetes client: %s", err)
	}
	k8sAPI := k8s.NewAPI(
		clientSet,
		k8s.Deploy,
		k8s.NS,
		k8s.Pod,
		k8s.RC,
		k8s.Svc,
	)

	server, lis, err := tap.NewServer(*addr, *tapPort, k8sAPI)
	if err != nil {
		log.Fatal(err.Error())
	}

	go func() {
		err := k8sAPI.Sync()
		if err != nil {
			log.Fatal(err.Error())
		}
	}()

	go func() {
		log.Println("starting gRPC server on", *addr)
		server.Serve(lis)
	}()

	go prometheus.NewMetricsServer(*metricsAddr)

	<-stop

	log.Println("shutting down gRPC server on", *addr)
	server.GracefulStop()
}
