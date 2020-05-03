package app

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/smuething/devicemonitor/config"
	"gopkg.in/yaml.v2"
)

type Configuration struct {
	config.ConfigBase

	LogLevel log.Level `yaml:"log_level,omitempty"`

	PProf struct {
		Enable  bool   `yaml:"enable,omitempty"`
		Address string `yaml:"address,omitempty"`
	} `yaml:"pprof,omitempty"`

	Paths struct {
		SpoolDir string `yaml:"spool_dir,omitempty"`
	} `yaml:"paths,omitempty"`

	Devices map[string]DeviceConfig `yaml:"devices,omitempty"`

	Printers map[string]PrinterConfig `yaml:"printers,omitempty"`
}

func (config *Configuration) Printer(name string) *PrinterConfig {
	if pc, ok := config.Printers[name]; ok {
		return &pc
	} else {
		return nil
	}
}

type DeviceConfig struct {
	Pos           int               `yaml:"pos,omitempty"`
	Device        string            `yaml:"device,omitempty"`
	Name          string            `yaml:"name,omitempty"`
	File          string            `yaml:"file,omitempty"`
	Timeout       time.Duration     `yaml:"timeout,omitempty"`
	Target        string            `yaml:"target,omitempty"`
	ExtendTimeout bool              `yaml:"extend_timeout,omitempty"`
	PrintViaPDF   bool              `yaml:"print_via_pdf,omitempty"`
	JobConfigs    map[string]string `yaml:"job_configs,omitempty"`
}

type PrinterConfig struct {
	Name       string               `yaml:"name,omitempty"`
	DefaultJob string               `yaml:"default_job,omitempty"`
	Jobs       map[string]JobConfig `yaml:"jobs,omitempty"`
}

type JobConfig struct {
	Pos              int    `yaml:"pos,omitempty"`
	Name             string `yaml:"name,omitempty"`
	Description      string `yaml:"description,omitempty"`
	PaperTrayPJLCode string `yaml:"paper_tray_pjl_code,omitempty"`
	Color            bool   `yaml:"color,omitempty"`
}

var wg *sync.WaitGroup = &sync.WaitGroup{}
var backgroundCtx context.Context
var ctx context.Context
var cancel context.CancelFunc
var cr *config.ConfigRepository

func init() {
	backgroundCtx = context.Background()
	ctx, cancel = context.WithCancel(backgroundCtx)
}

func LoadConfig(userConfigFile string, configFiles ...string) error {
	if cr != nil {
		return fmt.Errorf("Configuraiton already set up")
	}
	cr = config.New(
		func() config.LockingConfig { return &Configuration{} },
		userConfigFile,
		configFiles...,
	)
	cr.SetCreateMissing(true)
	cr.SetIgnoreMissing(true)
	err := cr.Load()
	config := Config()
	config.Lock()
	defer config.Unlock()
	log.Infof("Setting loglevel %s", config.LogLevel)
	log.SetLevel(config.LogLevel)
	return err
}

func ReloadConfig() error {
	err := cr.Load()
	config := Config()
	config.Lock()
	defer config.Unlock()
	if config.LogLevel != log.GetLevel() {
		log.Infof("Updating loglevel from % s to %s", log.GetLevel(), config.LogLevel)
		log.SetLevel(config.LogLevel)
	}
	return err
}

func Config() *Configuration {
	if cr == nil {
		panic("Cannot access config before calling LoadConfig()")
	}
	return cr.Config().(*Configuration)
}

func DumpConfig() {
	out, _ := yaml.Marshal(cr.Config())
	fmt.Println(string(out))
}

func SetConfigByPath(value interface{}, path ...string) error {
	return cr.Set(strings.ToLower(strings.Join(path, ".")), value)
}

func Go(f func()) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		f()
	}()
}

func GoWithError(f func() error) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := f()
		if err != nil {
			panic(err)
		}
	}()
}

func Shutdown(ctx context.Context) bool {

	cancel()

	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()

	select {
	case <-c:
		return true
	case <-ctx.Done():
		return false
	}
}

func Context() context.Context {
	return ctx
}

func ContextWithTimeout(timeout time.Duration, nested bool) (context.Context, context.CancelFunc) {
	if nested {
		return context.WithTimeout(ctx, timeout)
	} else {
		return context.WithTimeout(backgroundCtx, timeout)
	}
}
