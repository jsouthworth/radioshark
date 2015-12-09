package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/jsouthworth/radioshark"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
)

var (
	shark      string
	addr       string
	configfile string
	root       string
)

var errMethodNotAllowed = errors.New("method not allowed")

func init() {
	flag.StringVar(&shark, "shark", "", "RadioShark device to manage")
	flag.StringVar(&addr, "address", ":8080", "Address on which to listen")
	flag.StringVar(&configfile, "config", "",
		"Configuraton file location (default: /etc/rsharkd.<shark>.conf)")
	flag.StringVar(&root, "path", "/usr/share/rsharkd", "path to html")
}

type configError struct {
	errors []error
}

func (cerr *configError) Empty() bool {
	return len(cerr.errors) == 0
}

func (cerr *configError) Append(err error) {
	if err == nil {
		return
	}
	cerr.errors = append(cerr.errors, err)
}

func (cerr *configError) Error() string {
	errstrs := make([]string, 0, len(cerr.errors))
	for _, err := range cerr.errors {
		errstrs = append(errstrs, err.Error())
	}
	return strings.Join(errstrs, ", ")
}

type Config struct {
	Modulation       string `json:"modulation"`
	Frequency        string `json:"frequency"`
	BlueLEDIntensity uint8  `json:"blue-led-intensity"`
	BlueLEDPulseRate uint8  `json:"blue-led-pulse-rate"`
	RedLEDToggle     bool   `json:"red-led"`
}

func validateFrequency(modulation, frequency string) error {
	switch modulation {
	case "am", "AM":
		_, err := radioshark.ParseAMFrequency(frequency)
		return err
	case "fm", "FM":
		_, err := radioshark.ParseFMFrequency(frequency)
		return err
	}
	return nil
}

func writeConfig(file string, config *Config) error {
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return enc.Encode(config)
}

func readConfig(file string) (*Config, error) {
	var config *Config
	defConfig := &Config{
		Modulation:       "FM",
		Frequency:        "88.0",
		BlueLEDIntensity: 127,
		BlueLEDPulseRate: 0,
		RedLEDToggle:     false,
	}

	f, err := os.Open(file)
	if err != nil {
		log.Println(err)
		return defConfig, nil
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	err = dec.Decode(&config)
	if err != nil {
		log.Println(err)
		return defConfig, nil
	}
	return config, nil
}

type Server struct {
	sync.RWMutex
	configfile string
	cfg        *Config
	shark      *radioshark.RadioShark
}

func (srv *Server) setConfig(config *Config) {
	srv.cfg = config
}

func (srv *Server) Apply(config *Config) error {
	cerr := &configError{}

	err := srv.Validate(config)
	if err != nil {
		return err
	}

	curcfg := srv.Get()
	srv.setConfig(config)

	if curcfg.Frequency != config.Frequency ||
		curcfg.Modulation != config.Modulation {
		cerr.Append(srv.shark.SetFrequency(config.Modulation, config.Frequency))
	}

	cerr.Append(srv.shark.SetBlueLEDIntensity(config.BlueLEDIntensity))
	cerr.Append(srv.shark.SetBlueLEDPulse(config.BlueLEDPulseRate))
	cerr.Append(srv.shark.SetRedLED(config.RedLEDToggle))

	if cerr.Empty() {
		return writeConfig(srv.configfile, config)
	}
	return cerr
}

func (srv *Server) Validate(config *Config) error {
	cerr := &configError{}
	cerr.Append(radioshark.ValidateModulation(config.Modulation))
	cerr.Append(validateFrequency(config.Modulation, config.Frequency))
	cerr.Append(radioshark.ValidateBlueLEDIntensity(config.BlueLEDIntensity))
	cerr.Append(radioshark.ValidateBlueLEDPulse(config.BlueLEDPulseRate))
	if cerr.Empty() {
		return nil
	}
	return cerr
}

func (srv *Server) Get() *Config {
	var reply Config
	reply = *srv.cfg
	return &reply
}

func writeHTTPMethodNotAllowed(w http.ResponseWriter) {
	writeHTTPError(w, errMethodNotAllowed, http.StatusMethodNotAllowed)
}

func writeHTTPError(w http.ResponseWriter, err error, code int) {
	log.Println(err)
	resp := &struct {
		Error string `json:"error"`
	}{
		Error: err.Error(),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.Encode(resp)
}

func (srv *Server) httpGet(w http.ResponseWriter, req *http.Request) {
	srv.RLock()
	defer srv.RUnlock()
	switch req.Method {
	case "GET":
		enc := json.NewEncoder(w)
		enc.Encode(srv.Get())
	default:
		writeHTTPMethodNotAllowed(w)
	}
}

func (srv *Server) processApplyPOST(req *http.Request) (*Config, error) {
	config := srv.Get()
	reader, err := req.MultipartReader()
	if err != nil {
		return nil, err
	}

	form, err := reader.ReadForm(8192)
	if err != nil {
		return nil, err
	}

	modVals := form.Value["modulation"]
	if len(modVals) != 0 {
		config.Modulation = modVals[0]
	}

	freqVals := form.Value["frequency"]
	if len(freqVals) != 0 {
		config.Frequency = freqVals[0]
	}

	return config, nil
}

func (srv *Server) httpApply(w http.ResponseWriter, req *http.Request) {
	srv.RLock()
	defer srv.RUnlock()
	switch req.Method {
	case "PUT":
		var config *Config
		dec := json.NewDecoder(req.Body)
		dec.Decode(&config)
		err := srv.Apply(config)
		if err != nil {
			writeHTTPError(w, err, http.StatusBadRequest)
			return
		}
	case "POST":
		config, err := srv.processApplyPOST(req)
		if err != nil {
			writeHTTPError(w, err, http.StatusBadRequest)
			return
		}
		err = srv.Apply(config)
		if err != nil {
			writeHTTPError(w, err, http.StatusBadRequest)
			return
		}
	default:
		writeHTTPMethodNotAllowed(w)
	}
}

func (srv *Server) httpValidate(w http.ResponseWriter, req *http.Request) {
	srv.RLock()
	defer srv.RUnlock()
	switch req.Method {
	case "PUT":
		var config *Config
		dec := json.NewDecoder(req.Body)
		dec.Decode(&config)
		err := srv.Validate(config)
		if err != nil {
			writeHTTPError(w, err, http.StatusBadRequest)
			return
		}
	default:
		writeHTTPMethodNotAllowed(w)
	}
}

func NewServer(config string, shark string) (*Server, error) {
	cfg, err := readConfig(config)
	if err != nil {
		return nil, err
	}
	dev, err := radioshark.Open(shark)
	if err != nil {
		return nil, err
	}
	srv := &Server{
		configfile: config,
		cfg:        &Config{},
		shark:      dev,
	}
	err = srv.Apply(cfg)
	if err != nil {
		return nil, err
	}
	return srv, nil
}

func NewHTTPServeMux(config, shark, prefix string) (*http.ServeMux, error) {
	srv, err := NewServer(config, shark)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc(prefix+"/get", srv.httpGet)
	mux.HandleFunc(prefix+"/apply", srv.httpApply)
	mux.HandleFunc(prefix+"/validate", srv.httpValidate)
	return mux, nil
}

func die(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func parseFlags() error {
	flag.Parse()
	if shark == "" {
		return errors.New("the radioshark to manage must be supplied")
	}
	if configfile == "" {
		configfile = "/etc/rsharkd." + shark + ".conf"
	}
	return nil
}

func main() {
	die(parseFlags())
	exec.Command("modprobe", "-r", "radio_shark").Run()
	mux, err := NewHTTPServeMux(configfile, shark, "/config")
	die(err)
	mux.Handle("/", http.FileServer(http.Dir(root)))
	httpsrv := &http.Server{
		Handler: mux,
		Addr:    addr,
	}
	die(httpsrv.ListenAndServe())
}
