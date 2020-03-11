package config

import (
	"errors"
	"io/ioutil"
	"net/url"

	yaml "gopkg.in/yaml.v1"
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
}

type shortConfig struct {
	Target string
}

func Get(filename string) (conf *Config, err error) {
	conf = &Config{
		Concurrency:      2,
		Addr:             ":3001",
		UseCookies:       true,
		IgnoreAllQueries: false,
		IgnoreRobots:     false,
		Agent:            "foomo-walker",
	}
	yamlBytes, errRead := ioutil.ReadFile(filename)
	if errRead != nil {
		err = errRead
		return
	}
	errUnmarshal := yaml.Unmarshal(yamlBytes, &conf)
	if errUnmarshal != nil {
		err = errUnmarshal
		return
	}
	if conf.Target.BaseURL == "" {
		// try to get a short one
		shortConf := &shortConfig{}
		errUnmarshal := yaml.Unmarshal(yamlBytes, &shortConf)
		if errUnmarshal != nil {
			err = errUnmarshal
			return
		}
		if shortConf.Target != "" {
			conf.Target.BaseURL = shortConf.Target
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
