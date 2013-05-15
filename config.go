package main

import (
	"flag"
	"github.com/BurntSushi/toml"
	"github.com/placetime/datastore"
	"log"
	"os"
	"os/user"
	"path"
)

type Config struct {
	Web       WebConfig        `toml:"web"`
	Image     ImageConfig      `toml:"image"`
	Datastore datastore.Config `toml:"datastore"`
}

type WebConfig struct {
	Address string        `toml:"address"`
	Session SessionConfig `toml:"sessionlength"`
	Path    string        `toml:"path"`
	Admins  []string      `toml:"admins"`
}

type SessionConfig struct {
	Duration int    `toml:"duration"`
	Cookie   string `toml:"cookie"`
}

type ImageConfig struct {
	Path string `toml:"path"`
}

var (
	DefaultConfig Config = Config{
		Web: WebConfig{
			Address: "0.0.0.0:8081",
			Path:    "./assets",
			Admins:  []string{"@iand", "@daveg"},
			Session: SessionConfig{
				Duration: 86400 * 14,
				Cookie:   "ptsession",
			},
		},
		Datastore: datastore.DefaultConfig,
	}
)

func readConfig() {
	var configFile = ""
	var assetsDir = ""
	var imgDir = ""

	flag.StringVar(&configFile, "config", "", "configuration file to use")
	flag.StringVar(&assetsDir, "assets", "", "filesystem directory in which javascript/css/image assets are found")
	flag.StringVar(&imgDir, "images", "/var/opt/timescroll/img", "filesystem directory to store fetched images")
	flag.BoolVar(&doinit, "init", false, "re-initialize database (warning: will wipe eveything)")
	flag.Parse()

	config = DefaultConfig

	if configFile == "" {
		// Test home directory
		if u, err := user.Current(); err == nil {
			testFile := path.Join(u.HomeDir, ".placetime", "config")
			if _, err := os.Stat(testFile); err == nil {
				configFile = testFile
			}
		}
	}

	if configFile == "" {
		// Test /etc directory
		testFile := "/etc/placetime.conf"
		if _, err := os.Stat(testFile); err == nil {
			configFile = testFile
		}
	}

	if configFile != "" {
		configFile = path.Clean(configFile)
		if _, err := toml.DecodeFile(configFile, &config); err != nil {
			log.Printf("Could not read config file %s: %s", configFile, err.Error())
			os.Exit(1)
		}

		log.Printf("Reading configuration from %s", configFile)
	} else {
		log.Printf("Using default configuration")
	}

	// Some overrides from command line for backwards compatibility
	if assetsDir != "" && config.Web.Path != assetsDir {
		config.Web.Path = assetsDir
	}

	if imgDir != "" && config.Image.Path != imgDir {
		config.Image.Path = imgDir
	}

}

func checkEnvironment() {
	f, err := os.Open(config.Image.Path)
	if err != nil {
		log.Printf("Could not open image path %s: %s", config.Image.Path, err.Error())
		os.Exit(1)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		log.Printf("Could not stat image path %s: %s", config.Image.Path, err.Error())
		os.Exit(1)
	}

	if !fi.IsDir() {
		log.Printf("Image path is not a directory %s: %s", config.Image.Path, err.Error())
		os.Exit(1)
	}

}
