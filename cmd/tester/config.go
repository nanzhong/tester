package main

import (
	"fmt"
	"strings"
	"time"
)

type stringDuration time.Duration

func (d *stringDuration) UnmarshalJSON(b []byte) error {
	var sd stringDuration
	duration, err := time.ParseDuration(strings.Trim(string(b), `"`))
	if err != nil {
		return err
	}
	sd = stringDuration(duration)
	d = &sd
	return nil
}

func (d *stringDuration) MarshalJSON() (b []byte, err error) {
	return []byte(fmt.Sprintf(`"%s"`, time.Duration(*d).String())), nil
}

type runConfig struct {
	Path    string         `json:"path"`
	Timeout stringDuration `json:"timeout"`
}

type config struct {
	RunConfigs []runConfig `json:"run_configs"`
}
