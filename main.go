package main

import (
	"cgl.tideland.biz/applog"
	"code.google.com/p/gorilla/mux"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/iand/imgpick"
	"github.com/iand/salience"
	"github.com/kurrik/oauth1a"
	"github.com/kurrik/twittergo"
	"github.com/nranchev/go-libGeoIP"
	"github.com/placetime/datastore"
	"github.com/rcrowley/goagain"
	"html/template"
	"image/png"
	"io"
	"io/ioutil"
	mr "math/rand"
	"net"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var (
	configFile        = ""
	assetsDir         = ""
	imgDir            = ""
	config            Config
	templatesDir      = "./templates"
	newUserCookieName = "ptnewuser"
	doinit            = false
	cityDb            *libgeo.GeoIP
)

type GeoLocation struct {
	IPAddr      string  `json:"ip"`
	CountryCode string  `json:"countrycode"`
	CountryName string  `json:"countryname"`
	Region      string  `json:"region"`
	City        string  `json:"city"`
	PostalCode  string  `json:"postalcode"`
	Latitude    float32 `json:"latitude"`
	Longitude   float32 `json:"longitude"`
}

// TODO: Look into https://github.com/PuerkitoBio/ghost
// TODO: https://github.com/craigmj/gototp

func main() {
	mr.Seed(time.Now().UTC().UnixNano())
	runtime.GOMAXPROCS(runtime.NumCPU())

	flag.StringVar(&configFile, "config", "", "configuration file to use")
	flag.StringVar(&assetsDir, "assets", "", "filesystem directory in which javascript/css/image assets are found")
	flag.StringVar(&imgDir, "images", "/var/opt/timescroll/img", "filesystem directory to store fetched images")
	flag.BoolVar(&doinit, "init", false, "re-initialize database (warning: will wipe eveything)")
	flag.Parse()

	// go func() {
	// 	http.ListenAndServe("localhost:6060", nil)
	// }()

	Configure()

	if doinit {
		initData()
	}

	r := mux.NewRouter()

	r.PathPrefix("/policies").HandlerFunc(vocabRedirectHandler).Methods("GET", "HEAD")
	r.PathPrefix("/instant").HandlerFunc(vocabRedirectHandler).Methods("GET", "HEAD")
	r.PathPrefix("/interval").HandlerFunc(vocabRedirectHandler).Methods("GET", "HEAD")
	r.PathPrefix("/geopoint").HandlerFunc(vocabRedirectHandler).Methods("GET", "HEAD")
	r.PathPrefix("/technical").HandlerFunc(vocabRedirectHandler).Methods("GET", "HEAD")
	r.PathPrefix("/uridocs").HandlerFunc(vocabRedirectHandler).Methods("GET", "HEAD")
	r.PathPrefix("/changes.html").HandlerFunc(vocabRedirectHandler).Methods("GET", "HEAD")
	r.PathPrefix("/2003").HandlerFunc(vocabRedirectHandler).Methods("GET", "HEAD")
	r.PathPrefix("/2008").HandlerFunc(vocabRedirectHandler).Methods("GET", "HEAD")

	r.HandleFunc("/", homepageHandler).Methods("GET", "HEAD")

	r.HandleFunc("/timeline", timelineHandler).Methods("GET", "HEAD")
	r.HandleFunc("/item/{id:[0-9a-z]+}", itemHandler).Methods("GET", "HEAD")
	//r.HandleFunc("/-init", initHandler).Methods("GET", "HEAD")
	r.HandleFunc("/-admin", adminHandler).Methods("GET", "HEAD")

	r.HandleFunc("/-jsp", jsonSuggestedProfilesHandler).Methods("GET", "HEAD")
	r.HandleFunc("/-jpr", jsonProfileHandler).Methods("GET", "HEAD")
	r.HandleFunc("/-jit", jsonItemHandler).Methods("GET", "HEAD")
	r.HandleFunc("/-jtl", jsonTimelineHandler).Methods("GET", "HEAD")
	r.HandleFunc("/-jsp", jsonSuggestedProfilesHandler).Methods("GET", "HEAD")
	r.HandleFunc("/-jfollowers", jsonFollowersHandler).Methods("GET", "HEAD")
	r.HandleFunc("/-jfollowing", jsonFollowingHandler).Methods("GET", "HEAD")
	r.HandleFunc("/-jfeeds", jsonFeedsHandler).Methods("GET", "HEAD")
	r.HandleFunc("/-jflaggedprofiles", jsonFlaggedProfilesHandler).Methods("GET", "HEAD")
	r.HandleFunc("/-jsearch", jsonSearchHandler).Methods("GET", "HEAD")
	r.HandleFunc("/-jgeo", jsonGeoHandler).Methods("GET", "HEAD")
	r.HandleFunc("/-jdetect", jsonDetectHandler).Methods("GET", "HEAD")

	r.HandleFunc("/-tfollow", followHandler).Methods("POST")
	r.HandleFunc("/-tunfollow", unfollowHandler).Methods("POST")
	r.HandleFunc("/-tadd", addHandler).Methods("POST")
	r.HandleFunc("/-tpromote", promoteHandler).Methods("POST")
	r.HandleFunc("/-tdemote", demoteHandler).Methods("POST")
	r.HandleFunc("/-taddsuggest", addSuggestHandler).Methods("POST")
	r.HandleFunc("/-tremsuggest", remSuggestHandler).Methods("POST")
	r.HandleFunc("/-taddprofile", addProfileHandler).Methods("POST")
	r.HandleFunc("/-tupdateprofile", updateProfileHandler).Methods("POST")
	r.HandleFunc("/-tremprofile", removeProfileHandler).Methods("POST")
	r.HandleFunc("/-tflagprofile", flagProfileHandler).Methods("POST")

	r.HandleFunc("/-ping", pingHandler).Methods("GET")
	r.HandleFunc("/-session", sessionHandler).Methods("POST")
	r.HandleFunc("/-chksession", checkSessionHandler).Methods("GET")
	r.HandleFunc("/-twitter", twitterHandler).Methods("GET")
	r.HandleFunc("/-soauth", soauthHandler).Methods("GET")
	r.HandleFunc("/-tmpl", templatesHandler).Methods("GET")

	r.PathPrefix("/-assets/").HandlerFunc(assetsHandler).Methods("GET", "HEAD")
	r.PathPrefix("/-img/").HandlerFunc(imgHandler).Methods("GET", "HEAD")

	server := &http.Server{
		Addr:        config.Web.Address,
		Handler:     Log(r),
		ReadTimeout: 30 * time.Second,
	}

	listener, ppid, err := goagain.GetEnvs()
	_ = ppid
	if err != nil {
		// This is master process

		laddr, err := net.ResolveTCPAddr("tcp", config.Web.Address)
		if err != nil {
			applog.Errorf("Could not resolve TCP Address: %s", err.Error())
			os.Exit(1)
		}
		applog.Infof("Listening on %v", laddr)

		listener, err = net.ListenTCP("tcp", laddr)
		if nil != err {
			applog.Errorf("Could not listen on TCP Address: %s", err.Error())
			os.Exit(1)
		}

		go server.Serve(listener)

	} else {
		// This is spawned process

		// Resume listening and accepting connections in a new goroutine.
		applog.Infof("Resuming listening on %v", listener.Addr())
		go server.Serve(listener)

		// Kill the parent, now that the child has started successfully.
		applog.Infof("Killing parent pid %v", ppid)
		if err := goagain.KillParent(ppid); nil != err {
			applog.Errorf("Could not kill parent: %s", err.Error())
			os.Exit(1)
		}

	}

	// Block the main goroutine awaiting signals.
	if err := AwaitSignals(listener); nil != err {
		applog.Errorf("Error encountered in signal handler: %s", err.Error())
		os.Exit(1)
	}

	// Do whatever's necessary to ensure a graceful exit like waiting for
	// goroutines to terminate or a channel to become closed.

	// In this case, we'll simply stop listening and wait one second.
	if err := listener.Close(); nil != err {
		applog.Errorf("Error while closing listener: %s", err.Error())
		os.Exit(1)
	}
	time.Sleep(1 * time.Second)
}

func Configure() {
	readConfig()
	checkEnvironment()

	datastore.InitRedisStore(config.Datastore)

	var err error
	cityDb, err = libgeo.Load(config.Geo.CityDb)
	if err != nil {
		applog.Errorf("Error while opening citydb: %s", err.Error())
		os.Exit(1)
	}
	applog.Infof("Assets directory: %s", config.Web.Path)
	applog.Infof("Image directory: %s", config.Image.Path)
	applog.Infof("City database: %s", config.Geo.CityDb)

}

func AwaitSignals(l net.Listener) error {
	ch := make(chan os.Signal, 2)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGUSR2, syscall.SIGHUP)
	for {
		sig := <-ch
		applog.Debugf("Received signal: %s", sig.String())
		switch sig {

		// SIGHUP should reload configuration.

		case syscall.SIGHUP:
			applog.Infof("Re-reading configuration")
			Configure()

		// SIGQUIT should exit gracefully. However, Go doesn't seem
		// to like handling SIGQUIT (or any signal which dumps core by
		// default) at all so SIGTERM takes its place. How graceful
		// this exit is depends on what the program does after this
		// function returns control.
		case syscall.SIGTERM:
			return nil

		// TODO SIGUSR1 should reopen logs.

		// SIGUSR2 begins the process of restarting without dropping
		// the listener passed to this function.
		case syscall.SIGUSR2:
			err := goagain.Relaunch(l)
			if nil != err {
				return err
			}

		}
	}
	return nil // It'll never get here.
}

func Hostname() string {
	h, _ := os.Hostname()
	if h == "quickling" {
		return "127.0.0.1:8081"
	}
	return "placetime.com"
}

func Log(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		applog.Infof("%s %s %s", r.RemoteAddr, r.Method, r.URL)
		handler.ServeHTTP(w, r)
	})
}

func ErrorResponse(w http.ResponseWriter, r *http.Request, err error) {
	errcode, _ := RandomString(8)

	applog.Errorf("ERR503%s (%s) (code:%s)", err.Error(), r.URL, errcode)
	http.Error(w, fmt.Sprintf("An unexpected error occurred. (%s)", errcode), http.StatusInternalServerError)
}

func assetsHandler(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path[9:]
	p = path.Join(config.Web.Path, p)
	http.ServeFile(w, r, p)
	// w.Write([]byte(p))
}

func imgHandler(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path[6:]
	p = path.Join(config.Image.Path, p)
	http.ServeFile(w, r, p)
}

func homepageHandler(w http.ResponseWriter, r *http.Request) {
	templates := template.Must(template.ParseFiles(path.Join(config.Web.Path, "html/homepage.html")))

	err := templates.ExecuteTemplate(w, "homepage.html", nil)
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}
}

func timelineHandler(w http.ResponseWriter, r *http.Request) {
	templates := template.Must(template.ParseFiles(path.Join(config.Web.Path, "html/timeline.html")))

	err := templates.ExecuteTemplate(w, "timeline.html", nil)
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}
}

func itemHandler(w http.ResponseWriter, r *http.Request) {
	templates := template.Must(template.ParseFiles(path.Join(config.Web.Path, "html/item.html")))

	vars := mux.Vars(r)
	id := datastore.ItemIdType(vars["id"])

	s := datastore.NewRedisStore()
	defer s.Close()

	item, err := s.Item(id)
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}

	err = templates.ExecuteTemplate(w, "item.html", item)
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}
}

func jsonTimelineHandler(w http.ResponseWriter, r *http.Request) {
	sessionValid, _ := checkSession(w, r, false)
	if !sessionValid {
		return
	}

	r.ParseForm()
	_, exists := r.Form["pid"]
	if !exists {
		ErrorResponse(w, r, errors.New("pid parameter is required"))
		return
	}

	pidParam := datastore.PidType(r.FormValue("pid"))
	statusParam := r.FormValue("status")

	if statusParam != "m" {
		statusParam = "p"
	}

	beforeParam := r.FormValue("before")
	before, err := strconv.ParseInt(beforeParam, 10, 0)
	if err != nil {
		before = 0
	}

	afterParam := r.FormValue("after")
	after, err := strconv.ParseInt(afterParam, 10, 0)
	if err != nil {
		after = 0
	}

	var ts time.Time

	tsParam := r.FormValue("ts")
	tsVal, err := strconv.ParseInt(tsParam, 10, 64)
	if err != nil {
		ts = time.Now()
	} else {
		ts = time.Unix(0, tsVal)
	}

	s := datastore.NewRedisStore()
	defer s.Close()
	tl, err := s.TimelineRange(pidParam, statusParam, ts, int(before), int(after))
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}

	json, err := json.MarshalIndent(tl, "", "  ")
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/javascript")
	w.Write(json)
}

func jsonItemHandler(w http.ResponseWriter, r *http.Request) {
	sessionValid, _ := checkSession(w, r, false)
	if !sessionValid {
		return
	}

	id := datastore.ItemIdType(r.FormValue("id"))

	s := datastore.NewRedisStore()
	defer s.Close()

	item, err := s.Item(id)
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}

	json, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/javascript")
	w.Write(json)

}

func jsonSuggestedProfilesHandler(w http.ResponseWriter, r *http.Request) {
	sessionValid, _ := checkSession(w, r, false)
	if !sessionValid {
		return
	}

	loc := r.FormValue("loc")

	s := datastore.NewRedisStore()
	defer s.Close()

	plist, err := s.SuggestedProfiles(loc)
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}

	json, err := json.MarshalIndent(plist, "", "  ")
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/javascript")
	w.Write(json)

}

func jsonFollowersHandler(w http.ResponseWriter, r *http.Request) {
	sessionValid, _ := checkSession(w, r, false)
	if !sessionValid {
		return
	}

	pid := datastore.PidType(r.FormValue("pid"))

	countParam := r.FormValue("count")
	count, err := strconv.ParseInt(countParam, 10, 0)
	if err != nil {
		count = 10
	}

	startParam := r.FormValue("start")
	start, err := strconv.ParseInt(startParam, 10, 0)
	if err != nil {
		start = 0
	}

	s := datastore.NewRedisStore()
	defer s.Close()

	plist, err := s.Followers(pid, int(count), int(start))
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}

	json, err := json.MarshalIndent(plist, "", "  ")
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/javascript")
	w.Write(json)

}

func jsonFollowingHandler(w http.ResponseWriter, r *http.Request) {
	sessionValid, _ := checkSession(w, r, false)
	if !sessionValid {
		return
	}

	pid := datastore.PidType(r.FormValue("pid"))

	countParam := r.FormValue("count")
	count, err := strconv.ParseInt(countParam, 10, 0)
	if err != nil {
		count = 10
	}

	startParam := r.FormValue("start")
	start, err := strconv.ParseInt(startParam, 10, 0)
	if err != nil {
		start = 0
	}

	s := datastore.NewRedisStore()
	defer s.Close()

	plist, err := s.Following(pid, int(count), int(start))
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}

	json, err := json.MarshalIndent(plist, "", "  ")
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/javascript")
	w.Write(json)

}

func jsonProfileHandler(w http.ResponseWriter, r *http.Request) {
	sessionValid, _ := checkSession(w, r, false)
	if !sessionValid {
		return
	}

	pid := datastore.PidType(r.FormValue("pid"))

	s := datastore.NewRedisStore()
	defer s.Close()

	profile, err := s.Profile(pid)
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}

	json, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/javascript")
	w.Write(json)

}

func followHandler(w http.ResponseWriter, r *http.Request) {
	sessionValid, sessionPid := checkSession(w, r, false)
	if !sessionValid {
		return
	}

	pid := datastore.PidType(r.FormValue("pid"))
	if pid != sessionPid && !isAdmin(sessionPid) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	followpid := datastore.PidType(r.FormValue("followpid"))

	if followpid == pid {
		ErrorResponse(w, r, fmt.Errorf("Cannot follow self"))
		return
	}

	s := datastore.NewRedisStore()
	defer s.Close()

	err := s.Follow(pid, followpid)
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}
	fmt.Fprint(w, "ACK")
}

func unfollowHandler(w http.ResponseWriter, r *http.Request) {
	sessionValid, sessionPid := checkSession(w, r, false)
	if !sessionValid {
		return
	}

	pid := datastore.PidType(r.FormValue("pid"))
	if pid != sessionPid {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	followpid := datastore.PidType(r.FormValue("followpid"))
	s := datastore.NewRedisStore()
	defer s.Close()

	err := s.Unfollow(pid, followpid)
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}
	fmt.Fprint(w, "ACK")
}

func initData() {
	s := datastore.NewRedisStore()
	defer s.Close()

	applog.Infof("Resetting database")
	s.ResetAll()

	// applog.Infof("Adding profile for @iand")
	// err := s.AddProfile("@iand", "sunshine", "Ian", "Timefloes.", "", "", "nospam@iandavis.com", "", "", "", "", "")
	// if err != nil {
	// 	applog.Errorf("Could not add profile for @iand: %s", err.Error())
	// }
	// s.AddSuggestedProfile("@iand", "london")

	// applog.Infof("Adding profile for @daveg")
	// err = s.AddProfile("@daveg", "sunshine", "Dave", "", "", "", "", "", "", "", "", "")
	// if err != nil {
	// 	applog.Errorf("Could not add profile for @daveg: %s", err.Error())
	// }

	// applog.Infof("Adding profile for @nasa")
	// s.AddProfile("@nasa", "nasa", "Nasa Missions", "Upcoming NASA mission information.", "", "", "", "", "", "", "", "")

	// applog.Infof("Adding items for nasa")
	// s.AddItem("@nasa", parseKnownTime("1 Jan 2015"), "BepiColombo - Launch of ESA and ISAS Orbiter and Lander Missions to Mercury", "", "", "", "", 0)
	// s.AddItem("@nasa", parseKnownTime("26 Aug 2012"), "Dawn - Leaves asteroid Vesta, heads for asteroid 1 Ceres", "", "", "", "", 0)
	// s.AddItem("@nasa", parseKnownTime("1 Sep 2012"), "BepiColombo - Launch of ESA and ISAS Orbiter and Lander Missions to Mercury", "", "", "", "", 0)
	// s.AddItem("@nasa", parseKnownTime("1 Feb 2015"), "Dawn - Goes into orbit around asteroid 1 Ceres", "", "", "", "", 0)
	// s.AddItem("@nasa", parseKnownTime("14 Jul 2015"), "New Horizons - NASA mission reaches Pluto and Charon", "", "", "", "", 0)
	// s.AddItem("@nasa", parseKnownTime("1 Mar 2013"), "LADEE - Launch of NASA Orbiter to the Moon", "", "", "", "", 0)
	// s.AddItem("@nasa", parseKnownTime("1 Nov 2014"), "Philae - ESA Rosetta Lander touches down on Comet Churyumov-Gerasimenko", "", "", "", "", 0)
	// s.AddItem("@nasa", parseKnownTime("1 Nov 2013"), "MAVEN - Launch of Mars Orbiter", "", "", "", "", 0)
	// s.AddItem("@nasa", parseKnownTime("1 May 2014"), "Rosetta - ESA mission reaches Comet Churyumov-Gerasimenko", "", "", "", "", 0)
	// s.AddItem("@nasa", parseKnownTime("1 Jan 2014"), "Mars Sample Return Mission - Launch of NASA sample return mission to Mars", "", "", "", "", 0)
	// s.AddItem("@nasa", parseKnownTime("5 Apr 2231"), "Pluto - is passed by Neptune in distance from the Sun for the next 20 years", "", "", "", "", 0)

	// applog.Infof("Adding profile for @visitlondon")
	// err = s.AddProfile("@visitlondon", "sunshine", "visitlondon.com", "", "", "", "", "", "", "", "", "")
	// if err != nil {
	// 	applog.Errorf("Could not add profile for @visitlondon: %s", err.Error())
	// }

	// applog.Infof("Adding feed profile for londonsportsguide")
	// err = s.AddProfile("londonsportsguide", "sunshine", "Football in London - visitlondon.com", "", "http://feeds.visitlondon.com/LondonSportsGuide", "@visitlondon", "", "", "", "", "", "event")
	// if err != nil {
	// 	applog.Errorf("Could not add profile for londonsportsguide: %s", err.Error())
	// }

	// applog.Infof("Adding feed profile for londonartsguide")
	// err = s.AddProfile("londonartsguide", "sunshine", "London Arts Guide - visitlondon.com", "", "http://feeds.visitlondon.com/LondonArtsGuide", "@visitlondon", "", "", "", "", "", "event")
	// if err != nil {
	// 	applog.Errorf("Could not add profile for londonartsguide: %s", err.Error())
	// }

	// applog.Infof("Adding feed profile for londondanceguide")
	// err = s.AddProfile("londondanceguide", "sunshine", "London Dance Guide - visitlondon.com", "", "http://feeds.visitlondon.com/LondonDanceGuide", "@visitlondon", "", "", "", "", "", "event")
	// if err != nil {
	// 	applog.Errorf("Could not add profile for londondanceguide: %s", err.Error())
	// }

	// applog.Infof("Adding feed profile for o2shepherdsbushempire")
	// err = s.AddProfile("o2shepherdsbushempire", "sunshine", "O2 Shepherd's Bush Empire | Concert Dates and Tickets", "", "http://www.o2shepherdsbushempire.co.uk/RSS", "", "", "", "", "", "", "event")
	// if err != nil {
	// 	applog.Errorf("Could not add profile for o2shepherdsbushempire: %s", err.Error())
	// }

	applog.Infof("Adding follows for @iand")
	// s.Follow("@iand", "londonsportsguide")
	// s.Follow("@iand", "londonartsguide")
	// s.Follow("@iand", "londondanceguide")
	// s.Follow("@iand", "o2shepherdsbushempire")
	// s.Follow("@iand", "@nasa")
	s.Follow("@iand", "@daveg")

	applog.Infof("Adding follows for @daveg")
	// s.Follow("@daveg", "londonsportsguide")
	// s.Follow("@daveg", "londonartsguide")
	// s.Follow("@daveg", "londondanceguide")
	// s.Follow("@daveg", "o2shepherdsbushempire")
	// s.Follow("@daveg", "@nasa")
	s.Follow("@daveg", "@iand")

	applog.Infof("Initialisation complete")

}

// Redirects for content that used to be on this domain
func vocabRedirectHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "http://vocab.org/placetime"+r.URL.Path, http.StatusMovedPermanently)
}

func adminHandler(w http.ResponseWriter, r *http.Request) {
	sessionValid, sessionPid := checkSession(w, r, false)
	if !sessionValid {
		return
	}
	if !isAdmin(sessionPid) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	templates := template.Must(template.ParseFiles(path.Join(config.Web.Path, "html/admin.html")))

	err := templates.ExecuteTemplate(w, "admin.html", nil)
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}

}

func addHandler(w http.ResponseWriter, r *http.Request) {
	sessionValid, _ := checkSession(w, r, false)
	if !sessionValid {
		return
	}

	pid := datastore.PidType(r.FormValue("pid"))
	text := r.FormValue("text")
	link := r.FormValue("link")
	ets := r.FormValue("ets")
	event := r.FormValue("event")
	image := r.FormValue("image")
	media := r.FormValue("media")
	durationStr := r.FormValue("duration")

	if event == "" {
		event = ets
	}

	var duration int64
	var err error
	if durationStr != "" {
		duration, err = strconv.ParseInt(durationStr, 10, 32)
		if err != nil {
			ErrorResponse(w, r, fmt.Errorf("Duration was not an integer"))
			return
		}
	}

	applog.Debugf("Adding item pid: %s, text: %s, link: %s, event: %v, image: %s, media: %s", pid, text, link, event, image, media)

	s := datastore.NewRedisStore()
	defer s.Close()

	etsParsed := time.Unix(0, 0)

	eventNum, err := strconv.ParseInt(event, 10, 64)
	if err != nil {
		etsParsed, err = time.Parse(time.RFC3339, event)
		if err != nil {
			etsParsed, err = time.Parse("2006-01-02", event)
			if err != nil {
				etsParsed = time.Unix(0, 0)
			}
		}

	} else {
		etsParsed = time.Unix(eventNum, 0)
	}

	itemid, err := s.AddItem(pid, etsParsed, text, link, image, "", media, int(duration))
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}

	itemResponse(itemid, pid, w, r)
}

func promoteHandler(w http.ResponseWriter, r *http.Request) {
	sessionValid, sessionPid := checkSession(w, r, false)
	if !sessionValid {
		return
	}

	pid := datastore.PidType(r.FormValue("pid"))
	if pid != sessionPid && !isAdmin(sessionPid) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	id := datastore.ItemIdType(r.FormValue("id"))

	s := datastore.NewRedisStore()
	defer s.Close()

	err := s.Promote(pid, id)
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}

	itemResponse(id, pid, w, r)

}

func demoteHandler(w http.ResponseWriter, r *http.Request) {
	sessionValid, sessionPid := checkSession(w, r, false)
	if !sessionValid {
		return
	}

	pid := datastore.PidType(r.FormValue("pid"))
	if pid != sessionPid && !isAdmin(sessionPid) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	id := datastore.ItemIdType(r.FormValue("id"))
	s := datastore.NewRedisStore()
	defer s.Close()

	err := s.Demote(pid, id)
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}
	fmt.Fprint(w, "ACK")
}

func addSuggestHandler(w http.ResponseWriter, r *http.Request) {

	sessionValid, sessionPid := checkSession(w, r, false)
	if !sessionValid {
		return
	}
	if !isAdmin(sessionPid) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	pid := datastore.PidType(r.FormValue("pid"))
	loc := r.FormValue("loc")
	s := datastore.NewRedisStore()
	defer s.Close()

	err := s.AddSuggestedProfile(pid, loc)
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}
	fmt.Fprint(w, "ACK")
}

func remSuggestHandler(w http.ResponseWriter, r *http.Request) {
	sessionValid, sessionPid := checkSession(w, r, false)
	if !sessionValid {
		return
	}
	if !isAdmin(sessionPid) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	pid := datastore.PidType(r.FormValue("pid"))
	loc := r.FormValue("loc")
	s := datastore.NewRedisStore()
	defer s.Close()

	err := s.RemoveSuggestedProfile(pid, loc)
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}
	fmt.Fprint(w, "ACK")
}

func sessionHandler(w http.ResponseWriter, r *http.Request) {
	pid := datastore.PidType(strings.ToLower(r.FormValue("pid")))
	pwd := r.FormValue("pwd")

	s := datastore.NewRedisStore()
	defer s.Close()

	validPassword, err := s.VerifyPassword(pid, pwd)
	if err != nil || !validPassword {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	createSession(pid, w, r)
	fmt.Fprint(w, "")
}

func checkSession(w http.ResponseWriter, r *http.Request, silent bool) (bool, datastore.PidType) {
	var pid datastore.PidType
	valid := false

	cookie, err := r.Cookie(config.Web.Session.Cookie)
	if err == nil {
		parts := strings.Split(cookie.Value, "|")
		if len(parts) == 2 {
			pid = datastore.PidType(parts[0])
			sessionId, err := strconv.ParseInt(parts[1], 10, 64)
			s := datastore.NewRedisStore()
			defer s.Close()

			if err == nil {
				valid, err = s.ValidSession(pid, sessionId)
				if err != nil {
					ErrorResponse(w, r, err)
					return false, datastore.PidType("")
				}

				if valid {
					newSessionId, err := s.SessionId(pid)
					if err != nil {
						ErrorResponse(w, r, err)
						return false, datastore.PidType("")
					}

					value := fmt.Sprintf("%s|%d", pid, newSessionId)

					cookie := http.Cookie{Name: config.Web.Session.Cookie, Value: value, Path: "/", MaxAge: config.Web.Session.Duration}
					http.SetCookie(w, &cookie)
				}

			}
		}
	}

	if !silent && !valid {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}

	return valid, datastore.PidType(pid)

}
func createSession(pid datastore.PidType, w http.ResponseWriter, r *http.Request) {
	s := datastore.NewRedisStore()
	defer s.Close()

	sessionId, err := s.SessionId(pid)
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}

	value := fmt.Sprintf("%s|%d", pid, sessionId)

	cookie := http.Cookie{Name: config.Web.Session.Cookie, Value: value, Path: "/", MaxAge: 86400}
	http.SetCookie(w, &cookie)

}

func checkSessionHandler(w http.ResponseWriter, r *http.Request) {
	sessionValid, _ := checkSession(w, r, false)
	if !sessionValid {
		return
	}
}

func createOauthSession(w http.ResponseWriter, r *http.Request, userConfig *oauth1a.UserConfig) (string, error) {
	s := datastore.NewRedisStore()
	defer s.Close()

	data, err := json.Marshal(userConfig)
	if err != nil {
		return "", err
	}

	temporaryKey, _ := RandomString(12)

	err = s.SetOauthSessionData(temporaryKey, string(data))
	if err != nil {
		ErrorResponse(w, r, err)
		return "", err
	}
	cookie := http.Cookie{Name: "oatmp", Value: temporaryKey, Path: "/", MaxAge: 600}
	http.SetCookie(w, &cookie)
	return temporaryKey, nil
}

func readOauthSession(w http.ResponseWriter, r *http.Request) (*oauth1a.UserConfig, error) {
	s := datastore.NewRedisStore()
	defer s.Close()

	cookie, err := r.Cookie("oatmp")
	if err != nil {
		applog.Errorf("Could not read oatmp cookie: %s", err.Error())
		return nil, err
	}

	key := cookie.Value

	data, err := s.GetOauthSessionData(key)
	if err != nil {
		ErrorResponse(w, r, err)
		return nil, err
	}

	userConfig := &oauth1a.UserConfig{}

	err = json.Unmarshal([]byte(data), userConfig)

	if err != nil {
		applog.Errorf("Could not unmarshal oauth session data: %s", err.Error())
		return nil, err
	}
	return userConfig, nil
}

func addProfileHandler(w http.ResponseWriter, r *http.Request) {
	pid := datastore.PidType(r.FormValue("pid"))
	pwd := r.FormValue("pwd")
	name := r.FormValue("pname")
	feedurl := r.FormValue("feedurl")
	bio := r.FormValue("bio")
	parentpid := datastore.PidType(r.FormValue("parentpid"))
	email := r.FormValue("email")
	itemType := r.FormValue("itemtype")

	var err error
	if pwd == "" {
		pwd, err = RandomString(18)
		if err != nil {
			applog.Errorf("Could not generate password: %s", err.Error())
			ErrorResponse(w, r, err)
			return
		}
	}

	s := datastore.NewRedisStore()
	defer s.Close()

	err = s.AddProfile(pid, pwd, name, bio, feedurl, parentpid, email, "", "", "", "", itemType)
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}
	sessionValid, _ := checkSession(w, r, true)
	if !sessionValid {
		createSession(pid, w, r)
	}
	fmt.Fprint(w, "")
}

func updateProfileHandler(w http.ResponseWriter, r *http.Request) {
	sessionValid, _ := checkSession(w, r, false)
	if !sessionValid {
		return
	}

	r.ParseForm()

	_, exists := r.Form["pid"]
	if !exists {
		ErrorResponse(w, r, errors.New("pid parameter is required"))
		return
	}

	pid := datastore.PidType(r.FormValue("pid"))

	values := make(map[string]string, 0)

	for _, p := range datastore.ProfileProperties {
		if _, exists := r.Form[p]; exists {
			values[p] = r.FormValue(p)
		}
	}
	s := datastore.NewRedisStore()
	defer s.Close()

	err := s.UpdateProfile(pid, values)
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}
	fmt.Fprint(w, "")
}

func removeProfileHandler(w http.ResponseWriter, r *http.Request) {
	sessionValid, sessionPid := checkSession(w, r, false)
	if !sessionValid {
		return
	}

	pid := datastore.PidType(r.FormValue("pid"))
	if pid != sessionPid && !isAdmin(sessionPid) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	s := datastore.NewRedisStore()
	defer s.Close()

	err := s.RemoveProfile(pid)
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}
	fmt.Fprint(w, "")
}

func flagProfileHandler(w http.ResponseWriter, r *http.Request) {
	sessionValid, _ := checkSession(w, r, false)
	if !sessionValid {
		return
	}
	pid := datastore.PidType(r.FormValue("pid"))
	if pid == "" {
		ErrorResponse(w, r, errors.New("Missing required parameter 'pid'"))
		return

	}
	s := datastore.NewRedisStore()
	defer s.Close()

	err := s.FlagProfile(pid)
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}
	fmt.Fprint(w, "")
}

func twitterHandler(w http.ResponseWriter, r *http.Request) {
	service := OauthService()

	httpClient := new(http.Client)
	userConfig := &oauth1a.UserConfig{}
	err := userConfig.GetRequestToken(service, httpClient)
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}
	url, err := userConfig.GetAuthorizeURL(service)
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}

	createOauthSession(w, r, userConfig)
	applog.Infof("Redirecting user to %s", url)

	http.Redirect(w, r, url, http.StatusFound)

}
func soauthHandler(w http.ResponseWriter, r *http.Request) {
	service := OauthService()

	userConfig, err := readOauthSession(w, r)
	if err != nil {
		applog.Errorf("Could not read oauth session: %v", err)
		ErrorResponse(w, r, errors.New("Could not parse authorization"))
		return
	}

	token, verifier, err := userConfig.ParseAuthorize(r, service)
	if err != nil {
		applog.Errorf("Could not parse authorization: %v", err)
		ErrorResponse(w, r, errors.New("Could not parse authorization"))
		return
	}
	httpClient := new(http.Client)
	err = userConfig.GetAccessToken(token, verifier, service, httpClient)
	if err != nil {
		applog.Errorf("Error getting access token: %v", err)
		ErrorResponse(w, r, errors.New("Problem getting an access token"))
		return
	}

	screenName := userConfig.AccessValues.Get("screen_name")

	client := twittergo.NewClient(service.ClientConfig, userConfig)

	query := url.Values{}
	query.Set("screen_name", screenName)

	req, _ := http.NewRequest("GET", fmt.Sprintf("https://api.twitter.com/1.1/users/show.json?%s", query.Encode()), nil)
	response, err := client.SendRequest(req)
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}

	defer response.Body.Close()
	contents, err := ioutil.ReadAll(response.Body)
	if err != nil {
		applog.Errorf("Received an error while reading twitter response: %s", err)
		ErrorResponse(w, r, err)
		return
	}

	twitterUserData := &TwitterUser{}
	err = json.Unmarshal(contents, twitterUserData)

	if err != nil {
		ErrorResponse(w, r, err)
	}

	pid := datastore.PidType(fmt.Sprintf("@%s", screenName))

	s := datastore.NewRedisStore()
	defer s.Close()

	exists, err := s.ProfileExists(pid)
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}
	if !exists {
		pwd, err := RandomString(18)
		if err != nil {
			applog.Errorf("Could not generate password: %s", err.Error())
			ErrorResponse(w, r, err)
			return
		}

		err = s.AddProfile(pid, pwd, twitterUserData.Name, twitterUserData.Description, "", "", "", twitterUserData.Location, twitterUserData.Url, twitterUserData.ProfileImageUrl, twitterUserData.ProfileImageUrlHttps, "")
		if err != nil {
			ErrorResponse(w, r, err)
			return
		}

		cookie := http.Cookie{Name: newUserCookieName, Value: "true", Path: "/", MaxAge: config.Web.Session.Duration}
		http.SetCookie(w, &cookie)

	} else {
		values := make(map[string]string, 0)
		values["name"] = twitterUserData.Name
		values["bio"] = twitterUserData.Description
		values["url"] = twitterUserData.Url
		values["location"] = twitterUserData.Location
		values["profileimageurl"] = twitterUserData.ProfileImageUrl
		values["profileimageurlhttps"] = twitterUserData.ProfileImageUrlHttps

		s.UpdateProfile(pid, values)
	}

	createSession(pid, w, r)
	http.Redirect(w, r, "/timeline", http.StatusFound)

}

type TemplateMap map[string]string

func packageTemplates() (*TemplateMap, error) {
	filenames, err := filepath.Glob(fmt.Sprintf("%s/*.html", templatesDir))

	if err != nil {
		return nil, err
	}

	templateMap := make(TemplateMap, 0)

	for _, filename := range filenames {
		b, err := ioutil.ReadFile(filename)
		if err != nil {
			return nil, err
		}

		templateMap[filename[len(templatesDir)-1:len(filename)-5]] = string(b)
	}

	return &templateMap, nil
}

func templatesHandler(w http.ResponseWriter, r *http.Request) {
	tm, err := packageTemplates()

	if err != nil {
		ErrorResponse(w, r, err)
		return
	}

	json, err := json.MarshalIndent(tm, "", "  ")
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/javascript")

	w.Write([]byte(fmt.Sprintf("window.templates=%s;", json)))

}

func jsonFeedsHandler(w http.ResponseWriter, r *http.Request) {
	sessionValid, _ := checkSession(w, r, false)
	if !sessionValid {
		return
	}

	pid := datastore.PidType(r.FormValue("pid"))

	s := datastore.NewRedisStore()
	defer s.Close()

	flist, err := s.Feeds(pid)
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}

	json, err := json.MarshalIndent(flist, "", "  ")
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/javascript")
	w.Write(json)

}

func jsonSearchHandler(w http.ResponseWriter, r *http.Request) {
	sessionValid, pid := checkSession(w, r, false)
	if !sessionValid {
		return
	}

	var result SearchResults

	srch := r.FormValue("s")
	stype := r.FormValue("t")

	if srch == "" {
		ErrorResponse(w, r, errors.New("Invalid search entered"))
		return
	}

	if stype == "p" {
		result = ProfileSearch(srch)
	} else {
		if stype == "v" {
			result = VideoSearch(srch, pid)
		} else if stype == "a" {
			result = AudioSearch(srch, pid)
		} else if stype == "e" {
			result = EventSearch(srch, pid)
		} else {
			result = ItemSearch(srch, pid)
		}
		if items, ok := result.Results.(ItemSearchResults); ok {
			s := datastore.NewRedisStore()
			defer s.Close()

			fitems := make(FormattedItemSearchResults, 0)

			for _, item := range items {
				s.SaveItem(item, config.Search.Lifetime)

				fitem, err := s.FormatItem(item, 0, item.Pid)
				if err != nil {
					continue
				}
				fitems = append(fitems, fitem)
			}

			result.Results = fitems
		}

	}

	json, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/javascript")
	w.Write(json)

}

func isAdmin(pid datastore.PidType) bool {
	for _, v := range config.Web.Admins {
		if datastore.PidType(v) == pid {
			return true
		}
	}

	return false
}

func parseKnownTime(t string) time.Time {
	ret, _ := time.Parse("_2 Jan 2006", t)
	return ret
}

func randomString(length int) string {
	b := make([]byte, length)
	rand.Read(b)
	en := base64.URLEncoding
	d := make([]byte, en.EncodedLen(len(b)))
	en.Encode(d, b)
	return string(d)
}

func jsonFlaggedProfilesHandler(w http.ResponseWriter, r *http.Request) {
	sessionValid, sessionPid := checkSession(w, r, false)
	if !sessionValid {
		return
	}

	if !isAdmin(sessionPid) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	startParam := r.FormValue("start")
	start, err := strconv.ParseInt(startParam, 10, 0)
	if err != nil {
		start = 0
	}

	countParam := r.FormValue("count")
	count, err := strconv.ParseInt(countParam, 10, 0)
	if err != nil {
		count = 10
	}

	s := datastore.NewRedisStore()
	defer s.Close()
	profiles, err := s.FlaggedProfiles(int(start), int(count))
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}

	json, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/javascript")
	w.Write(json)
}

func OauthService() *oauth1a.Service {
	return &oauth1a.Service{
		RequestURL:   "https://api.twitter.com/oauth/request_token",
		AuthorizeURL: "https://api.twitter.com/oauth/authorize",
		AccessURL:    "https://api.twitter.com/oauth/access_token",
		ClientConfig: &oauth1a.ClientConfig{
			ConsumerKey:    config.Twitter.OAuthConsumerKey,
			ConsumerSecret: config.Twitter.OAuthConsumerSecret,
			CallbackURL:    fmt.Sprintf("http://%s/-soauth", Hostname()),
		},
		Signer: new(oauth1a.HmacSha1Signer),
	}
}

func pingHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "PONG")
}

func itemResponse(id datastore.ItemIdType, pid datastore.PidType, w http.ResponseWriter, r *http.Request) {
	s := datastore.NewRedisStore()
	defer s.Close()

	item, err := s.Item(id)
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}

	items, err := s.ItemInTimeline(item, pid, "m")
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}

	json, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/javascript")
	w.Write(json)
}

func jsonGeoHandler(w http.ResponseWriter, r *http.Request) {
	sessionValid, _ := checkSession(w, r, false)
	if !sessionValid {
		return
	}

	ipAddr := r.FormValue("ip")

	if ipAddr == "" {

		if v, exists := r.Header["X-Forwarded-For"]; exists {
			ipAddr = v[0]
		} else {
			ipAddr = strings.Split(r.RemoteAddr, ":")[0]

		}
	}

	locformatted := GeoLocation{
		IPAddr: ipAddr,
	}

	loc := cityDb.GetLocationByIP(ipAddr)

	if loc != nil {
		locformatted.CountryCode = loc.CountryCode
		locformatted.CountryName = loc.CountryName
		locformatted.Region = loc.Region
		locformatted.City = loc.City
		locformatted.PostalCode = loc.PostalCode
		locformatted.Latitude = loc.Latitude
		locformatted.Longitude = loc.Longitude
	}

	json, err := json.MarshalIndent(locformatted, "", "  ")
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/javascript")
	w.Write(json)
}

func jsonDetectHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	_, exists := r.Form["url"]
	if !exists {
		ErrorResponse(w, r, errors.New("url parameter is required"))
		return
	}

	url := r.FormValue("url")
	best := r.FormValue("best")

	println(url)

	mediaUrl, title, imageUrls, err := imgpick.FindMedia(url)

	if err != nil {
		ErrorResponse(w, r, err)
		return
	}

	type DetectionResult struct {
		Title      string              `json:"title"`
		Url        string              `json:"url"`
		Images     []string            `json:"images,omitempty"`
		Alternates []imgpick.ImageInfo `json:"alternates,omitempty"`
		Media      string              `json:"media"`
		BestImage  string              `json:"bestImage"`
	}

	var data DetectionResult

	var bestImageFilename string

	if best == "1" {
		best, images, err := imgpick.SelectBestImage(url, imageUrls)

		if best.Img == nil || err != nil {
			ErrorResponse(w, r, err)
			return
		}

		imgOut := salience.Crop(best.Img, 460, 160)

		hasher := md5.New()
		io.WriteString(hasher, url)
		id := fmt.Sprintf("%x", hasher.Sum(nil))

		bestImageFilename = fmt.Sprintf("%s.png", id)

		foutName := path.Join(config.Image.Path, bestImageFilename)

		fout, err := os.OpenFile(foutName, os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			ErrorResponse(w, r, err)
			return
		}

		if err = png.Encode(fout, imgOut); err != nil {
			ErrorResponse(w, r, err)
			return
		}

		filteredImages := make([]imgpick.ImageInfo, 0)
		for _, i := range images {
			if i.Url != best.Url {
				filteredImages = append(filteredImages, i)
			}
		}

		data = DetectionResult{
			Title:      title,
			Url:        url,
			Alternates: filteredImages,
			Media:      mediaUrl,
			BestImage:  bestImageFilename,
		}

	} else {

		data = DetectionResult{
			Title:     title,
			Url:       url,
			Images:    imageUrls,
			Media:     mediaUrl,
			BestImage: bestImageFilename,
		}
	}

	json, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/javascript")
	w.Write(json)

}
