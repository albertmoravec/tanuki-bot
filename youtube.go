package main

import (
	"bufio"
	"fmt"
	"github.com/rylio/ytdl"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"
	"google.golang.org/api/youtube/v3"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
)

type YoutubeItem struct {
	Video   *ytdl.VideoInfo
	ytdlCmd *exec.Cmd
}

func (yt *YoutubeItem) Play() io.Reader {
	//TODO YTDL returning early
	/*reader, writer := io.Pipe()
	format := yt.Video.Formats.Extremes(ytdl.FormatAudioBitrateKey, true)[0]
	go func() {
		url, err := yt.Video.GetDownloadURL(format)
		if err != nil {
			log.Println("error: " + err.Error())
		}

		resp, err := http.Get(url.String())
		if err != nil {
			log.Println("error: " + err.Error())
		}
		defer resp.Body.Close()

		_, err = io.Copy(writer, resp.Body)
		if err != nil {
			log.Println("error: " + err.Error())
		}
		log.Println("Download returned")
	}()*/

	yt.ytdlCmd = exec.Command("ytdl", "-s", "-f", "best-audio", "-o", "-", yt.Video.ID)
	yt.ytdlCmd.Stderr = os.Stderr
	youtubedlOut, err := yt.ytdlCmd.StdoutPipe()
	if err != nil {
		fmt.Println("YTDL StdoutPipe Error:", err)
		return nil
	}

	err = yt.ytdlCmd.Start()
	if err != nil {
		fmt.Println("YTDL StdoutPipe Error:", err)
		return nil
	}
	return bufio.NewReaderSize(youtubedlOut, 65536)

	//return io.Reader(bufio.NewReaderSize(reader, 65536))
}

func (yt *YoutubeItem) Stop() {
	if yt.ytdlCmd.Process != nil {
		yt.ytdlCmd.Process.Kill()
	}
}

func (yt *YoutubeItem) GetInfo() ItemInfo {
	return ItemInfo{
		Title:    yt.Video.Title,
		Duration: yt.Video.Duration.String(),
	}
}

func LoadYoutubeAPIConfig(filePath string) (*jwt.Config, error) {
	token, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	return google.JWTConfigFromJSON(token, youtube.YoutubeScope)
}
