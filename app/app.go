package app

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/smuething/devicemonitor/config"
)

type Configuration struct {
	config.ConfigBase

	LogLevel string `yaml:"log_level,omitempty"`

	PProf struct {
		Enable  bool   `yaml:"enable,omitempty"`
		Address string `yaml:"address,omitempty"`
	} `yaml:"pprof,omitempty"`

	Paths struct {
		SpoolDir string `yaml:"spool_dir,omitempty"`
	} `yaml:"paths,omitempty"`

	Devices map[string]DeviceConfig `yaml:"devices,omitempty"`
}

type DeviceConfig struct {
	Pos     int           `yaml:"pos,omitempty"`
	Device  string        `yaml:"device,omitempty"`
	Name    string        `yaml:"name,omitempty"`
	File    string        `yaml:"file,omitempty"`
	Timeout time.Duration `yaml:"timeout,omitempty"`
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
	return cr.Load()
}

func ReloadConfig() error {
	return cr.Load()
}

func Config() *Configuration {
	if cr == nil {
		panic("Cannot access config before calling LoadConfig()")
	}
	return cr.Config().(*Configuration)
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
