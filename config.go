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
	Twitter   TwitterConfig    `toml:"twitter"`
	Geo       GeoConfig        `toml:"geo"`
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
	Songkick SongkickConfig `toml:"songkick"`
	Lastfm   LastfmConfig   `toml:"lastm"`
	Spotify  SpotifyConfig  `toml:"spotify"`
	Youtube  YoutubeConfig  `toml:"youtube"`
}

type EventfulConfig struct {
	AppKey string `toml:"appkey"`
	Pid    string `toml:"pid"`
}

type YoutubeConfig struct {
	Pid string `toml:"pid"`
}

type SpotifyConfig struct {
	Pid string `toml:"pid"`
}

type SongkickConfig struct {
	AppKey string `toml:"appkey"`
}

type LastfmConfig struct {
	APIKey string `toml:"apikey"`
	Secret string `toml:"secret"`
}

type TwitterConfig struct {
	OAuthConsumerKey    string `toml:"consumerkey"`
	OAuthConsumerSecret string `toml:"consumersecret"`
}

type GeoConfig struct {
	CityDb string `toml:"citydb"`
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
				Pid:    "eventful",
			},
			Songkick: SongkickConfig{
				AppKey: "KVAGcEtQWTuMJJUX",
			},
			Lastfm: LastfmConfig{
				APIKey: "e8bc090bb831d109fcee1b1450e87bd3",
				Secret: "646a5a3b50d83ee8d43a4085ec5cc9e7",
			},
			Youtube: YoutubeConfig{
				Pid: "youtube",
			},
			Spotify: SpotifyConfig{
				Pid: "spotify",
			},
		},
		Twitter: TwitterConfig{
			OAuthConsumerKey:    "Fnky4HZ8z4NsOxRniTvCA",
			OAuthConsumerSecret: "iv9q7CTYfrls05eFlhyEkPpHcJqseSWpbDx8GIyGvg",
		},
		Geo: GeoConfig{
			CityDb: "data/GeoLiteCity.dat",
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

	f, err = os.Open(config.Web.Path)
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

	f, err = os.Open(config.Geo.CityDb)
	if err != nil {
		applog.Errorf("Could not open city database %s: %s", config.Geo.CityDb, err.Error())
		os.Exit(1)
	}
	defer f.Close()
	fi, err = f.Stat()
	if err != nil {
		applog.Errorf("Could not stat city database %s: %s", config.Geo.CityDb, err.Error())
		os.Exit(1)
	}

	if !fi.Mode().IsRegular() {
		applog.Errorf("City databas path is not a file %s: %s", config.Geo.CityDb, err.Error())
		os.Exit(1)
	}

}
