package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"sync"

	"github.com/fatih/color"
	tld "github.com/jpillora/go-tld"
)

func scanS3FileSlow(fileURLs []string, bucketURL string) error {
	var errors []error

	//BELOW CODE BLOCK IS FOR ARRANGING BUCKETLOOT OUTPUT
	var bucketScanRes bucketLootResStruct
	bucketScanRes.BucketUrl = bucketURL

	for _, fileURL := range fileURLs {
		var bucketLootAsset bucketlootAssetStruct
		var bucketLootSecret bucketlootSecretStruct
		var bucketLootKeyword bucketlootKeywordStruct
		var keywordDisc int
		// Make HTTP request to S3 bucket URL
		resp, err := http.Get(fileURL)
		if err != nil {
			errors = append(errors, fmt.Errorf("error making HTTP request to S3 bucket file URL: %v", err))
			continue
		}
		defer resp.Body.Close()

		// Check response status code for errors
		if resp.StatusCode != http.StatusOK {
			if resp.StatusCode == http.StatusNotFound {
				errors = append(errors, fmt.Errorf("s3 bucket file not found: %s", fileURL))
			} else if resp.StatusCode == http.StatusForbidden {
				errors = append(errors, fmt.Errorf("s3 bucket file is private: %s", fileURL))
			} else {
				errors = append(errors, fmt.Errorf("unexpected response status code from S3 bucket file URL: %d: %s", resp.StatusCode, fileURL))
			}
			continue
		}

		// Read response body
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			errors = append(errors, fmt.Errorf("error reading response body from S3 bucket file URL: %v: %s", err, fileURL))
			continue
		}

		// Parse HTML to scan S3 Files
		//Extract Secrets
		for regName, regValue := range regexList {
			reg := regexp.MustCompile(regValue)
			if reg.MatchString(string(body)) {
				fmt.Printf("Discovered %v in %s\n", color.RedString("SECRET["+regName+"]"), fileURL)
				bucketLootSecret.Name = regName
				bucketLootSecret.URL = fileURL
				bucketScanRes.Secrets = append(bucketScanRes.Secrets, bucketLootSecret)
				bucketLootSecret.Name = ""
				bucketLootSecret.URL = ""
			}
		}

		//Extract URLs
		extURLs := urlRE.FindAllString(string(body), -1) // EXTRACT URLS FROM FILE
		urlAssets = append(urlAssets, extURLs...)        // APPEND TO ENTIRE URL LIST
		if len(extURLs) > 0 {
			fmt.Printf("Discovered %v in %s\n", color.BlueString("URL(s)"), fileURL)
		}

		//Extract Domains - Subdomains
		for _, u := range extURLs { // USE URLS EXTRACTED FROM FILE FOR SCANNING
			bucketLootAsset.URL = u
			asset, err := tld.Parse(u)
			if err == nil {
				domAssets = append(domAssets, asset.Domain+"."+asset.TLD) // APPEND TO ENTIRE DOMAIN LIST
				bucketLootAsset.Domain = asset.Domain + "." + asset.TLD
				if asset.Subdomain != "" { // IF THE ASSET URL HAS A SUBDOMAIN
					subAssets = append(subAssets, asset.Subdomain+"."+asset.Domain+"."+asset.TLD) // APPEND TO ENTIRE DOMAIN LIST
					bucketLootAsset.Subdomain = asset.Subdomain + "." + asset.Domain + "." + asset.TLD
				}
			}
			bucketScanRes.Assets = append(bucketScanRes.Assets, bucketLootAsset)
			bucketLootAsset.URL = ""
			bucketLootAsset.Domain = ""
			bucketLootAsset.Subdomain = ""
		}

		// SEARCH FOR USER DEFINED KEYWORDS
		for _, keyword := range scanKeywords {
			keywordRe := regexp.MustCompile(keyword)
			if keywordRe.MatchString(fileURL) {
				bucketLootKeyword.Keyword = keyword
				bucketLootKeyword.URL = fileURL
				bucketLootKeyword.Type = "FilePath"
				bucketScanRes.Keywords = append(bucketScanRes.Keywords, bucketLootKeyword)
				keywordDisc = 1
			}
			if keywordRe.MatchString(string(body)) {
				bucketLootKeyword.Keyword = keyword
				bucketLootKeyword.URL = fileURL
				bucketLootKeyword.Type = "FileContent"
				bucketScanRes.Keywords = append(bucketScanRes.Keywords, bucketLootKeyword)
				keywordDisc = 1
			}
		}

		if keywordDisc == 1 {
			fmt.Printf("Discovered %v in %s\n", color.GreenString("Keyword(s)"), fileURL)
		}
	}
	bucketlootOutput.Results = append(bucketlootOutput.Results, bucketScanRes)
	if len(errors) > 0 {
		for _, err := range errors {
			if *errorLogging {
				bucketlootOutput.Errors = append(bucketlootOutput.Errors, string(err.Error()))
			}
		}
	}

	return nil
}

func scanS3FilesFast(fileURLs []string, bucketURL string) error {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errors []error

	bucketScanRes := bucketLootResStruct{
		BucketUrl: bucketURL,
	}

	for _, fileURL := range fileURLs {
		wg.Add(1)

		go func(url string) {
			defer wg.Done()

			var (
				bucketLootAsset   bucketlootAssetStruct
				bucketLootSecret  bucketlootSecretStruct
				bucketLootKeyword bucketlootKeywordStruct
				keywordDisc       int
			)

			resp, err := http.Get(url)
			if err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("error making HTTP request to S3 bucket file URL: %v", err))
				mu.Unlock()
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				mu.Lock()
				if resp.StatusCode == http.StatusNotFound {
					errors = append(errors, fmt.Errorf("s3 bucket file not found: %s", url))
				} else if resp.StatusCode == http.StatusForbidden {
					errors = append(errors, fmt.Errorf("s3 bucket file is private: %s", url))
				} else {
					errors = append(errors, fmt.Errorf("unexpected response status code from S3 bucket file URL: %d: %s", resp.StatusCode, url))
				}
				mu.Unlock()
				return
			}

			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("error reading response body from S3 bucket file URL: %v: %s", err, url))
				mu.Unlock()
				return
			}

			for regName, regValue := range regexList {
				reg := regexp.MustCompile(regValue)
				if reg.MatchString(string(body)) {
					fmt.Printf("Discovered %v in %s\n", color.RedString("SECRET["+regName+"]"), url)
					bucketLootSecret.Name = regName
					bucketLootSecret.URL = url
					mu.Lock()
					bucketScanRes.Secrets = append(bucketScanRes.Secrets, bucketLootSecret)
					mu.Unlock()
					bucketLootSecret.Name = ""
					bucketLootSecret.URL = ""
				}
			}

			extURLs := urlRE.FindAllString(string(body), -1)
			mu.Lock()
			urlAssets = append(urlAssets, extURLs...)
			mu.Unlock()
			if len(extURLs) > 0 {
				fmt.Printf("Discovered %v in %s\n", color.BlueString("URL(s)"), url)
			}

			for _, u := range extURLs {
				bucketLootAsset.URL = u
				asset, err := tld.Parse(u)
				if err == nil {
					mu.Lock()
					domAssets = append(domAssets, asset.Domain+"."+asset.TLD)
					bucketLootAsset.Domain = asset.Domain + "." + asset.TLD
					if asset.Subdomain != "" {
						subAssets = append(subAssets, asset.Subdomain+"."+asset.Domain+"."+asset.TLD)
						bucketLootAsset.Subdomain = asset.Subdomain + "." + asset.Domain + "." + asset.TLD
					}
					mu.Unlock()
				}
				mu.Lock()
				bucketScanRes.Assets = append(bucketScanRes.Assets, bucketLootAsset)
				mu.Unlock()
				bucketLootAsset.URL = ""
				bucketLootAsset.Domain = ""
				bucketLootAsset.Subdomain = ""
			}

			for _, keyword := range scanKeywords {
				keywordRe := regexp.MustCompile(keyword)
				if keywordRe.MatchString(url) {
					bucketLootKeyword.Keyword = keyword
					bucketLootKeyword.URL = url
					bucketLootKeyword.Type = "FilePath"
					mu.Lock()
					bucketScanRes.Keywords = append(bucketScanRes.Keywords, bucketLootKeyword)
					keywordDisc = 1
					mu.Unlock()
				}
				if keywordRe.MatchString(string(body)) {
					bucketLootKeyword.Keyword = keyword
					bucketLootKeyword.URL = url
					bucketLootKeyword.Type = "FileContent"
					mu.Lock()
					bucketScanRes.Keywords = append(bucketScanRes.Keywords, bucketLootKeyword)
					keywordDisc = 1
					mu.Unlock()
				}
			}

			if keywordDisc == 1 {
				fmt.Printf("Discovered %v in %s\n", color.GreenString("Keyword(s)"), url)
			}
		}(fileURL)
	}

	wg.Wait()

	bucketlootOutput.Results = append(bucketlootOutput.Results, bucketScanRes)
	if len(errors) > 0 {
		for _, err := range errors {
			if *errorLogging {
				bucketlootOutput.Errors = append(bucketlootOutput.Errors, string(err.Error()))
			}
		}
	}

	return nil
}
