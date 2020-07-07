package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"net/rpc"
	"time"

	"github.com/kardianos/service"
	"github.com/smuething/devicemonitor/printing"
)

var logger service.Logger

// Program structures.
//  Define Start and Stop methods.
type program struct {
	exit        chan struct{}
	printServer *printing.PrintServer
	rpcServer   *rpc.Server
	httpServer  *http.Server
}

func (p *program) Start(s service.Service) error {
	if service.Interactive() {
		logger.Info("Running in terminal.")
	} else {
		logger.Info("Running under service manager.")
	}
	p.exit = make(chan struct{})
	// Start should not block. Do the actual work async.
	go p.run()
	return nil
}

func (p *program) run() error {
	var err error = nil
	logger.Infof("Running %v.", service.Platform())

	p.printServer = &printing.PrintServer{}

	p.rpcServer = rpc.NewServer()

	if err = p.rpcServer.Register(p.printServer); err != nil {
		log.Fatal("Error registering RPC API: ", err)
	}

	p.httpServer = &http.Server{
		Addr:         "0.0.0.0:8787",
		WriteTimeout: time.Second * 10,
		ReadTimeout:  time.Second * 10,
		IdleTimeout:  time.Second * 15,
		Handler:      p.rpcServer,
	}

	if err = p.httpServer.ListenAndServe(); err != nil {
		logger.Error(err)
	}

	return err
}

func (p *program) Stop(s service.Service) error {
	// Any work in Stop should be quick, usually a few seconds at most.
	logger.Info("Shutting down AM Environment Setup Server")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	return p.httpServer.Shutdown(ctx)
}

// Service setup.
//   Define service config.
//   Create the service.
//   Setup the logger.
//   Handle service controls (optional).
//   Run the service.
func main() {
	svcFlag := flag.String("service", "", "Control the system service.")
	flag.Parse()

	options := make(service.KeyValue)
	options["Restart"] = "on-success"
	svcConfig := &service.Config{
		Name:        "DeviceMonitorPrintServer",
		DisplayName: "DeviceMonitor RPC Print Server",
		Description: "This service receives print jobs from DeviceMonitor and forwards them to actual printers.",
		Option:      options,
	}

	prg := &program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatal(err)
	}
	errs := make(chan error, 5)
	logger, err = s.Logger(errs)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		for {
			err := <-errs
			if err != nil {
				log.Print(err)
			}
		}
	}()

	if len(*svcFlag) != 0 {
		err := service.Control(s, *svcFlag)
		if err != nil {
			log.Printf("Valid actions: %q\n", service.ControlAction)
			log.Fatal(err)
		}
		return
	}
	err = s.Run()
	if err != nil {
		logger.Error(err)
	}
}
