package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"sync"

	"github.com/fatih/structs"
	"github.com/imdario/mergo"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cast"
	"gopkg.in/yaml.v3"
)

type LockingConfig interface {
	sync.Locker
	setMutex(mutex *sync.RWMutex)
}

type ConfigBase struct {
	m *sync.RWMutex
}

func (c *ConfigBase) setMutex(mutex *sync.RWMutex) {
	c.m = mutex
}

func (c *ConfigBase) Lock() {
	c.m.RLock()
}

func (c *ConfigBase) Unlock() {
	c.m.RUnlock()
}

type MakeConfigFunc func() LockingConfig

// A special map that will interpret all map keys as strings when unmarshaling from YAML
type MapStr map[string]interface{}

func (m *MapStr) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		panic(fmt.Errorf("Unexpected YAML node kind"))
	}
	// children are represented as successive pairs of nodes, with the first node
	// containing the name of the child and the second one containing the value
	for i := 0; i < len(n.Content); i += 2 {
		k := n.Content[i]
		v := n.Content[i+1]
		switch v.Kind {
		case yaml.MappingNode:
			cv := MapStr{}
			if err := v.Decode(&cv); err != nil {
				return err
			}
			(*m)[k.Value] = cv
		case yaml.ScalarNode:
			var cv interface{}
			if err := v.Decode(&cv); err != nil {
				return err
			}
			(*m)[k.Value] = cv
		default:
			panic(fmt.Errorf("Unsupported node: %v", v))
		}
	}
	return nil
}

type ConfigRepository struct {
	m              sync.RWMutex
	ignoreMissing  bool
	createMissing  bool
	makeConfig     MakeConfigFunc
	config         LockingConfig
	configFiles    []string
	fixedConfig    LockingConfig
	fixedConfigMap map[string]interface{}
	userConfigFile string
	userConfig     MapStr
}

func (cr *ConfigRepository) IsIgnoreMissing() bool {
	return cr.ignoreMissing
}

func (cr *ConfigRepository) SetIgnoreMissing(ignoreMissing bool) {
	cr.ignoreMissing = ignoreMissing
}

func (cr *ConfigRepository) IsCreateMissing() bool {
	return cr.createMissing
}

func (cr *ConfigRepository) SetCreateMissing(createMissing bool) {
	cr.createMissing = createMissing
}

func New(makeConfig MakeConfigFunc, userConfigFile string, configFiles ...string) *ConfigRepository {
	return &ConfigRepository{
		makeConfig:     makeConfig,
		configFiles:    configFiles,
		userConfigFile: userConfigFile,
	}
}

func (cr *ConfigRepository) toMap(s interface{}) map[string]interface{} {
	struct_ := structs.New(s)
	struct_.TagName = "yaml"
	m := struct_.Map()
	return m
}

func (cr *ConfigRepository) Load() error {
	cr.fixedConfig = cr.makeConfig()
	for _, file := range cr.configFiles {
		data, err := ioutil.ReadFile(file)
		if err != nil {
			if os.IsNotExist(err) && cr.ignoreMissing {
				log.Warnf("config file not found: %s", file)
				if cr.createMissing {
					log.Infof("creating empty config file: %s", file)
					f, err := os.Create(file)
					if err != nil {
						log.Errorf("Failed to create %s: %s", f, err)
					}
					f.Close()
				}
				continue
			}
			return err
		}
		fileConfig := cr.makeConfig()
		if err = yaml.Unmarshal(data, fileConfig); err != nil {
			return err
		}
		if err = mergo.Merge(cr.fixedConfig, fileConfig, mergo.WithOverride, mergo.WithTypeCheck); err != nil {
			return err
		}
	}
	cr.fixedConfigMap = cr.toMap(cr.fixedConfig)
	cr.userConfig = MapStr{}
	data, err := ioutil.ReadFile(cr.userConfigFile)
	if err != nil {
		if os.IsNotExist(err) && cr.ignoreMissing {
			log.Warnf("configuration file not found: %s", cr.userConfigFile)
			if cr.createMissing {
				log.Infof("creating empty config file: %s", cr.userConfigFile)
				f, err := os.Create(cr.userConfigFile)
				if err != nil {
					log.Errorf("Failed to create %s: %s", f, cr.userConfigFile)
				}
				f.Close()
			}
		} else {
			return err
		}
	}
	if err = yaml.Unmarshal(data, &cr.userConfig); err != nil {
		return err
	}
	cr.m.Lock()
	defer cr.m.Unlock()
	return cr.rebuildConfig()
}

func (cr *ConfigRepository) rebuildConfig() error {
	cr.config = cr.makeConfig()
	if err := mergo.Merge(cr.config, cr.fixedConfig, mergo.WithOverride, mergo.WithTypeCheck); err != nil {
		return err
	}
	data, err := yaml.Marshal(cr.userConfig)
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	userConfig := cr.makeConfig()
	if err = yaml.Unmarshal(data, userConfig); err != nil {
		return err
	}
	if err := mergo.Merge(cr.config, userConfig, mergo.WithOverride); err != nil {
		return err
	}
	cr.config.setMutex(&cr.m)
	return nil
}

func normalizeKey(key string) string {
	return key
	// return strings.Replace(key, "_", "", -1)
}

func getNestedMapValue(m map[string]interface{}, key string) interface{} {
	return getNestedMapValueSlice(m, strings.Split(key, "."))
}

func getNestedMapValueSlice(m map[string]interface{}, key []string) interface{} {
	switch len(key) {
	case 0:
		return m
	case 1:
		return m[normalizeKey(key[0])]
	default:
		iv := m[normalizeKey(key[0])]
		switch v := iv.(type) {
		case map[interface{}]interface{}:
			return getNestedMapValueSlice(cast.ToStringMap(v), key[1:])
		case map[string]interface{}:
			return getNestedMapValueSlice(v, key[1:])
		default:
			return nil
		}
	}
}

func pruneNestedMapKey(m MapStr, key string) error {
	return pruneNestedMapKeySlice(m, strings.Split(key, "."))
}

func pruneNestedMapKeySlice(m MapStr, key []string) error {
	switch len(key) {
	case 1:
		delete(m, key[0])
		return nil
	default:
		iv := m[key[0]]
		if v, ok := iv.(MapStr); ok {
			if err := pruneNestedMapKeySlice(v, key[1:]); err != nil {
				return err
			}
			if len(v) == 0 {
				delete(m, key[0])
			}
			return nil
		} else {
			return fmt.Errorf("Could not delete key, invalid type for non-empty key")
		}
	}
}

func setNestedMapKey(m MapStr, key string, value interface{}) error {
	return setNestedMapKeySlice(m, strings.Split(key, "."), 0, value)
}

func setNestedMapKeySlice(m MapStr, key []string, i int, value interface{}) error {
	if i < 0 || i >= len(key) {
		panic(fmt.Errorf("Should never get called with empty key"))
	}
	if i == len(key)-1 {
		m[key[i]] = value
		return nil
	} else {
		iv := m[key[i]]
		switch v := iv.(type) {
		case MapStr:
			return setNestedMapKeySlice(v, key, i+1, value)
		case nil:
			nm := MapStr{}
			m[key[i]] = nm
			return setNestedMapKeySlice(nm, key, i+1, value)
		default:
			return fmt.Errorf("Cannot set key %s, non-map entry at %s", strings.Join(key, "."), strings.Join(key[:i+1], "."))
		}
	}
}

func (cr *ConfigRepository) Set(key string, value interface{}) error {
	cr.m.Lock()
	defer cr.m.Unlock()
	fixedValue := getNestedMapValue(cr.fixedConfigMap, key)
	if fixedValue == nil {
		typ := reflect.TypeOf(value)
		switch typ {
		case reflect.TypeOf(true):
			fixedValue = false
		case reflect.TypeOf(""):
			fixedValue = ""
		}
	}
	if value == nil || value == fixedValue {
		if err := pruneNestedMapKey(cr.userConfig, key); err != nil {
			return err
		}
	} else {
		if err := setNestedMapKey(cr.userConfig, key, value); err != nil {
			return err
		}
	}
	err := cr.rebuildConfig()
	if err != nil {
		return err
	}
	f, err := os.Create(cr.userConfigFile + ".swp")
	if err != nil {
		return err
	}
	defer f.Close()
	enc := yaml.NewEncoder(f)
	defer enc.Close()
	if err = enc.Encode(cr.userConfig); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(cr.userConfigFile+".swp", cr.userConfigFile)
}

func (cr *ConfigRepository) Config() interface{} {
	return cr.config
}
