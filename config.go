package main

import (
	"gopkg.in/yaml.v2"
	"io/ioutil"
)

type Configuration struct {
	//Discord settings
	Token       string `yaml:"token"`       // required, with Bot prefix
	Guild       string `yaml:"guild"`       // required
	TextChannel string `yaml:"textChannel"` // required, will listen to commands in this channel
	Owner       string `yaml:"owner"`       // optional, won't let you set permissions and use admin commands

	//Service settings
	YoutubeAPIKey string `yaml:"ytApiKey"`
}

func (config *Configuration) Load(configPath string) {
	configFile, _ := ioutil.ReadFile(configPath)
	if configFile != nil {
		yaml.Unmarshal(configFile, &config)
	}
}

func (config Configuration) Validate() bool { // this is rather a placeholder for a meaningful implementation
	return validateString(config.Token) && validateString(config.Guild) && validateString(config.TextChannel)
}

func validateString(input string) bool {
	return input != ""
}
