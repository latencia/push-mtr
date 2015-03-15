package main

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/headzoo/surf"
	"github.com/headzoo/surf/browser"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"time"
)

type WgetResult struct {
	TimeStart    time.Time `json:"time_start"`
	HTMLTime     int64     `json:"html_time"`
	TotalTime    int64     `json:"total_time"`
	Bytes        int64     `json:"bytes"`
	DownloadDir  string    `json"string"`
	LinkedAssets int       `json:"linked_assets"`
	URL          string    `json:"url"`
}

func downloadAsset(dir string, asset interface{}, ch *browser.AsyncDownloadChannel) error {
	filename := path.Join(dir, asset.(browser.Assetable).Url().Path)
	os.MkdirAll(filepath.Dir(filename), 0755)

	fout, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("Error creating temp file: %s\n", err)
	}
	asset.(browser.Downloadable).DownloadAsync(fout, *ch)

	return nil
}

// Download an HTML page and the linked assets
//
// Setting the downloadDir to an empty string, a temporary directory
// is created and the assets are downloaded there.
//
// Setting clean to true removes downloadDir after the assets have
// been downloaded.
func Wget(url string, downloadDir string, clean bool) (res WgetResult, err error) {

	res.URL = url
	res.TimeStart = time.Now()
	bow := surf.NewBrowser()

	if err = bow.Open(url); err != nil {
		return WgetResult{}, fmt.Errorf("Error opening URL: %s\n", err)
	}

	// time it takes to download the HTML
	res.HTMLTime = time.Since(res.TimeStart).Nanoseconds()

	images := bow.Images()[:]
	css := bow.Stylesheets()
	scripts := bow.Scripts()
	downloadChannel := make(browser.AsyncDownloadChannel, 1)
	res.LinkedAssets = len(images) + len(css) + len(scripts)

	if downloadDir == "" {
		if res.DownloadDir, err = ioutil.TempDir(downloadDir, ""); err != nil {
			return WgetResult{}, fmt.Errorf("Wget: Error creating tempdir: %s\n", err)
		}
		if clean {
			defer os.RemoveAll(res.DownloadDir)
		}
	} else {
		res.DownloadDir = downloadDir
	}

	for _, asset := range images {
		downloadAsset(res.DownloadDir, asset, &downloadChannel)
	}

	for _, asset := range scripts {
		downloadAsset(res.DownloadDir, asset, &downloadChannel)
	}

	for _, asset := range css {
		downloadAsset(res.DownloadDir, asset, &downloadChannel)
	}

	// Now we wait for each download to complete.
	for i := 0; i < res.LinkedAssets; i++ {
		result := <-downloadChannel
		if result.Error != nil {
			log.Errorf("Error download '%s'. %s\n", result.Asset.Url(), result.Error)
		} else {
			log.Debugf("Downloaded '%s'.\n", result.Asset.Url())
		}
		res.Bytes += result.Size
	}

	res.TotalTime = time.Since(res.TimeStart).Nanoseconds()
	log.Debugf("Total time: %0.3f\n", float64(res.TotalTime)/float64(time.Second))
	log.Debugf("Assets downloaded: %d", res.LinkedAssets)
	log.Debugf("Time to URL %s, %0.3f\n", url, float64(res.HTMLTime)/float64(time.Second))

	return res, nil
}

//func main() {
//	fmt.Println(Wget("https://github.com", "", true))
//}
