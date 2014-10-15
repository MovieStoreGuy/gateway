package main

import (
	"flag"
	"github.com/golang/glog"
	"github.com/signalfuse/signalfxproxy/config"
	"github.com/signalfuse/signalfxproxy/core"
	"github.com/signalfuse/signalfxproxy/forwarder"
	"github.com/signalfuse/signalfxproxy/listener"
	"io/ioutil"
	"os"
	"strconv"
)

func writePidFile(pidFileName *string) error {
	pid := os.Getpid()
	err := ioutil.WriteFile(*pidFileName, []byte(strconv.FormatInt(int64(pid), 10)), os.FileMode(0644))
	if err != nil {
		return err
	}
	return nil
}

func main() {
	configFileName := flag.String("configfile", "sf/sfdbproxy.conf", "Name of the db proxy configuration file")
	pidFileName := flag.String("signalfxproxypid", "signalfxproxy.pid", "Name of the file to store the PID in")
	flag.Parse()

	writePidFile(pidFileName)
	glog.Infof("Looking for config file %s\n", *configFileName)

	loadedConfig, err := config.LoadConfig(*configFileName)
	if err != nil {
		glog.Fatalln("Unable to load config: ", err)
		return
	}

	glog.Infof("Config is %s\n", loadedConfig)
	allForwarders := []core.DatapointStreamingAPI{}
	allStatKeepers := []core.StatKeeper{}
	for _, forwardConfig := range loadedConfig.ForwardTo {
		loader, ok := forwarder.AllForwarderLoaders[forwardConfig.Type]
		if !ok {
			glog.Fatalf("Unknown loader type: %s", forwardConfig.Type)
			return
		}
		forwarder, err := loader(forwardConfig)
		if err != nil {
			glog.Fatalf("Unable to loader forwarder: %s", err)
			return
		}
		allForwarders = append(allForwarders, forwarder)
		allStatKeepers = append(allStatKeepers, forwarder)
	}
	multiplexer, err := forwarder.NewStreamingDatapointDemultiplexer(allForwarders)
	if err != nil {
		glog.Fatalf("Unable to loader multiplexer: %s", err)
		return
	}
	allStatKeepers = append(allStatKeepers, multiplexer)

	allListeners := make([]listener.DatapointListener, len(loadedConfig.ListenFrom))
	for _, forwardConfig := range loadedConfig.ListenFrom {
		loader, ok := listener.AllListenerLoaders[forwardConfig.Type]
		if !ok {
			glog.Fatalf("Unknown loader type: %s", forwardConfig.Type)
			return
		}
		listener, err := loader(multiplexer, forwardConfig)
		if err != nil {
			glog.Fatalf("Unable to loader listener: %s", err)
			return
		}
		if listener == nil {
			glog.Fatalf("Got nil listener from %s", forwardConfig.Type)
		}
		allListeners = append(allListeners, listener)
		allStatKeepers = append(allStatKeepers, listener)
	}

	glog.Infof("Setup done.  Blocking!\n")
	stopChannel := make(chan bool, 2)
	if loadedConfig.StatsDelayDuration != nil && *loadedConfig.StatsDelayDuration != 0 {
		go core.DrainStatsThread(*loadedConfig.StatsDelayDuration, allForwarders, allStatKeepers, stopChannel)
	} else {
		glog.Infof("Skipping stat keeping")
	}

	// TODO: Replace with something more graceful that allows us to shutdown?
	select {}
}
