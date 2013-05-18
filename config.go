package main

import (
	"cgl.tideland.biz/applog"
	"github.com/BurntSushi/toml"
	"github.com/placetime/datastore"
	"os"
	"os/user"
	"path"
)

type Config struct {
	Web       WebConfig        `toml:"web"`
	Image     ImageConfig      `toml:"image"`
	Datastore datastore.Config `toml:"datastore"`
	Search    SearchConfig     `toml:"search"`
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

type SearchConfig struct {
	Lifetime int            `toml:"lifetime"`
	Timeout  int            `toml:"timeout"`
	Eventful EventfulConfig `toml:"eventful"`
}

type EventfulConfig struct {
	AppKey string `toml:"appkey"`
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
		Image: ImageConfig{
			Path: "/var/opt/timescroll/img",
		},
		Datastore: datastore.DefaultConfig,
		Search: SearchConfig{
			Lifetime: 600,
			Timeout:  15000,
			Eventful: EventfulConfig{
				AppKey: "h6xD8gZFzDK5m498",
			},
		},
	}
)

func readConfig() {

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
			applog.Errorf("Could not read config file %s: %s", configFile, err.Error())
			os.Exit(1)
		}

		applog.Infof("Reading configuration from %s", configFile)
	} else {
		applog.Infof("Using default configuration")
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
		applog.Errorf("Could not open image path %s: %s", config.Image.Path, err.Error())
		os.Exit(1)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		applog.Errorf("Could not stat image path %s: %s", config.Image.Path, err.Error())
		os.Exit(1)
	}

	if !fi.IsDir() {
		applog.Errorf("Image path is not a directory %s: %s", config.Image.Path, err.Error())
		os.Exit(1)
	}

	f, err = os.Open(config.Image.Path)
	if err != nil {
		applog.Errorf("Could not open web assets path %s: %s", config.Web.Path, err.Error())
		os.Exit(1)
	}
	defer f.Close()

	fi, err = f.Stat()
	if err != nil {
		applog.Errorf("Could not stat web assets path %s: %s", config.Web.Path, err.Error())
		os.Exit(1)
	}

	if !fi.IsDir() {
		applog.Errorf("Web assets path is not a directory %s: %s", config.Web.Path, err.Error())
		os.Exit(1)
	}

}
