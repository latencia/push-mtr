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
	TimeStart   time.Time
	HTMLTime    int64
	TotalTime   int64
	Bytes       int64
	DownloadDir string
}

func downloadAsset(dir string, asset interface{}, ch *browser.AsyncDownloadChannel) error {
	filename := path.Join(dir, asset.(browser.Assetable).Url().Path)
	os.MkdirAll(filepath.Dir(filename), 0755)

	fout, err := os.Create(filename)
	if err != nil {
		return err
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
func Wget(url string, downloadDir string, clean bool) (res WgetResult) {

	bow := surf.NewBrowser()
	res.TimeStart = time.Now()

	err := bow.Open(url)
	if err != nil {
		panic(err)
	}

	// time it takes to download the HTML
	ttURL := time.Since(res.TimeStart)

	images := bow.Images()
	css := bow.Stylesheets()
	scripts := bow.Scripts()
	downloadChannel := make(browser.AsyncDownloadChannel)
	totalAssets := len(images) + len(css) + len(scripts)

	if downloadDir == "" {
		res.DownloadDir, err = ioutil.TempDir(downloadDir, "")
		defer func() {
			if clean {
				fmt.Println("foooo")
				os.RemoveAll(res.DownloadDir)
			}
		}()
		if err != nil {
			panic("Wget: Error creating tempdir")
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
	for i := 0; i < totalAssets; i++ {
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
	log.Debugf("Assets downloaded: %d", totalAssets)
	log.Debugf("Time to URL %s, %0.3f\n", url, float64(ttURL)/float64(time.Second))

	return res
}

//func main() {
//	fmt.Println(Wget("https://github.com", "", true))
//}
