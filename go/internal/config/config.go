package config

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"strings"
	"time"

	yaml "gopkg.in/yaml.v3"
)

type CheckContent struct {
	Title       bool
	Description bool
	Tags        []string
}

type Target struct {
	BaseURL string
	Paths   []string
}
type config struct {
	Concurrency       int
	Addr              string
	Target            interface{}
	Ignore            []string
	IgnoreQueriesWith []string
	IgnoreAllQueries  bool
	UseCookies        bool
	Depth             int
	Paging            bool
	IgnoreRobots      bool
	GroupHeader       string
	Agent             string
	SchemaRoot        string
	Interval          time.Duration
	Offset            time.Duration
}

// type shortConfig struct {
// 	Target string
// }

type Config struct {
	Concurrency       int
	Addr              string
	Target            Target
	Ignore            []string
	IgnoreQueriesWith []string
	IgnoreAllQueries  bool
	UseCookies        bool
	Depth             int
	Paging            bool
	IgnoreRobots      bool
	GroupHeader       string
	Agent             string
	SchemaRoot        string
	// Interval between the starts of full scrape cycles, aligned to the wall
	// clock - 0 restarts immediately after a cycle completed
	Interval time.Duration
	// Offset shifts the cycle boundaries, eg interval 24h offset 3h => daily at 03:00
	Offset time.Duration
}

func Get(filename string) (conf *Config, err error) {
	yamlBytes, errRead := ioutil.ReadFile(filename)
	if errRead != nil {
		err = errRead
		return
	}
	return Load(yamlBytes)
}

func Load(yamlBytes []byte) (conf *Config, err error) {
	cnf := &config{
		Concurrency:      2,
		Addr:             ":3001",
		UseCookies:       true,
		IgnoreAllQueries: false,
		IgnoreRobots:     false,
		Agent:            "foomo-walker",
	}
	errUnmarshal := yaml.Unmarshal(yamlBytes, &cnf)
	if errUnmarshal != nil {
		err = errUnmarshal
		return
	}

	conf = &Config{
		Concurrency:       cnf.Concurrency,
		Addr:              cnf.Addr,
		Ignore:            cnf.Ignore,
		IgnoreQueriesWith: cnf.IgnoreQueriesWith,
		IgnoreAllQueries:  cnf.IgnoreAllQueries,
		UseCookies:        cnf.UseCookies,
		Depth:             cnf.Depth,
		Paging:            cnf.Paging,
		IgnoreRobots:      cnf.IgnoreRobots,
		GroupHeader:       cnf.GroupHeader,
		Agent:             cnf.Agent,
		SchemaRoot:        cnf.SchemaRoot,
		Interval:          cnf.Interval,
		Offset:            cnf.Offset,
	}
	if conf.Interval < 0 {
		return nil, errors.New("interval must not be negative")
	}
	if conf.Offset < 0 || (conf.Offset > 0 && conf.Offset >= conf.Interval) {
		return nil, errors.New("offset must be >= 0 and smaller than interval")
	}

	switch cnf.Target.(type) {
	case string:
		conf.Target.BaseURL = cnf.Target.(string)
	case map[string]interface{}:
		for key, v := range cnf.Target.(map[string]interface{}) {
			key = strings.ToLower(key)
			switch key {
			case "baseurl":
				switch v.(type) {
				case string:
					conf.Target.BaseURL = v.(string)
				default:
					return nil, errors.New("illegal type for target.BaseURL")
				}
			case "paths":
				switch v.(type) {
				case []interface{}:
					for _, p := range v.([]interface{}) {
						conf.Target.Paths = append(conf.Target.Paths, fmt.Sprint(p))
					}
				default:
					return nil, errors.New("illegal type for target.Paths")
				}

			}
		}
	}
	if len(conf.Target.Paths) == 0 {
		baseURL, errURL := url.Parse(conf.Target.BaseURL)
		if errURL != nil {
			return nil, errors.New("target/target.baseurl can not be parsed: " + errURL.Error())
		}
		if baseURL.Path == "" {
			conf.Target.Paths = []string{"/"}
		} else {
			conf.Target.Paths = []string{baseURL.Path}
			baseURL.Path = ""
			conf.Target.BaseURL = baseURL.String()
		}
	}
	if conf.Target.BaseURL == "" {
		return nil, errors.New("target base url must not be empty")
	}
	return
}
