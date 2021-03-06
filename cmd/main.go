package main

import (
	"log"
	"os"
	"os/user"
	"os/signal"
	"syscall"

	"github.com/ghostdb/ghostdb-cache-node/server/ghost_http"
	"github.com/ghostdb/ghostdb-cache-node/store/base"
	"github.com/ghostdb/ghostdb-cache-node/store/persistence"
	"github.com/ghostdb/ghostdb-cache-node/store/monitor"
	"github.com/ghostdb/ghostdb-cache-node/config"
	"github.com/ghostdb/ghostdb-cache-node/system_monitor"
)

// Node configuration file
var conf config.Configuration

// Main cache object
var store *base.Store

// Schedulers
var sysMetricsScheduler *system_monitor.SysMetricsScheduler

func init() {
	conf = config.InitializeConfiguration()

	usr, _ := user.Current()
	configPath := usr.HomeDir
	log.Println("LOG PATH: "+configPath)

	err := os.Mkdir(configPath+"/ghostdb", 0777)
	if err != nil {
		log.Printf("Failed to create GhostDB configuration directory")
	}

	// Create sysMetrics and appMetrics logfiles if they do not exist
	sysMetricsFile, err := os.OpenFile(configPath + system_monitor.SysMetricsLogFilename, os.O_WRONLY | os.O_CREATE | os.O_APPEND, 0777)
	if err != nil {
		log.Fatalf("Failed to create or read sysMetrics log file: %s", err.Error())
		panic(err)
	}
	defer sysMetricsFile.Close()

	appMetricsFile, err := os.OpenFile(configPath + monitor.AppMetricsLogFilePath, os.O_WRONLY | os.O_CREATE | os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("Failed to create or read appMetrics log file: %s", err.Error())
	}
	defer appMetricsFile.Close()


	store = base.NewStore("LRU") // FUTURE: Read store type from config
	store.BuildStore(conf)


	// Build the cache from a snapshot if snaps enabled.
	// If the snapshot does not exist, then build a new cache.
	if conf.SnapshotEnabled {
		if _, err := os.Stat(configPath + persistence.GetSnapshotFilename()); err == nil {
			bytes := persistence.ReadSnapshot(conf.EnableEncryption, conf.Passphrase)
			store.BuildStoreFromSnapshot(bytes)
			log.Println("successfully booted from snapshot...")
		} else { 
			log.Println("successfully booted new cache...")
		}
	} else {
		if conf.PersistenceAOF {
			if ok, _ := persistence.AofExists(); ok {
				persistence.RebootAof(&store.Cache, conf.AofMaxBytes)
			}
			store.BuildStoreFromAof()
			persistence.BootAOF(&store.Cache, conf.AofMaxBytes)
			log.Println("successfully booted from AOF...")
		}
		log.Println("successfully booted new cache...")
	}

	store.RunStore()

	sysMetricsScheduler = system_monitor.NewSysMetricsScheduler(conf.SysMetricInterval)
}

func main() {
	go system_monitor.StartSysMetrics(sysMetricsScheduler)
	log.Println("successfully started sysMetrics monitor...")
	ghost_http.NodeConfig(store)
	log.Println("successfully started GhostDB Node server...")
	log.Println("GhostDB started successfully...")

	t := make(chan os.Signal)
	signal.Notify(t, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-t
		log.Println("exiting...")
		os.Exit(1)
	}()

	ghost_http.Router()
}
