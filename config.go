package main

import (
	"gopkg.in/yaml.v2"
	"io/ioutil"
)

type config struct {
	//Discord settings
	Token        string `yaml:"token"`        // required, with Bot prefix
	Guild        string `yaml:"guild"`        // required
	TextChannel  string `yaml:"textChannel"`  // required, will listen to commands in this channel
	VoiceChannel string `yaml:"voiceChannel"` // required, will join by default
	Owner        string `yaml:"owner"`        // optional, won't let you set permissions and use admin commands

	//Encoder settings
	FFmpegPath string `yaml:"ffmpegPath"` // optional, will look for FFmpeg executable in environment or its folder if not specified

	//Service settings
	YoutubeAPIKey string `yaml:"ytApiKey"`
}

func (config *config) Load(configPath string) {
	configFile, _ := ioutil.ReadFile(configPath)
	if configFile != nil {
		yaml.Unmarshal(configFile, &Config)
	}
}

func (config config) Validate() bool { // this is rather a placeholder for a meaningful implementation
	return validateString(config.Token) && validateString(config.Guild) && validateString(config.TextChannel) && validateString(config.VoiceChannel)
}

func validateString(input string) bool {
	return input != ""
}
