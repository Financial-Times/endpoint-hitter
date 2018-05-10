package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"github.com/dchest/uniuri"
	"github.com/jawher/mow.cli"
	"github.com/lytics/logrus"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	appDescription = "Small application that is able to hit in parallel a requested endpoint - llogging whether the request was successful."
	maxRetries     = 3
)

var (
	client http.Client
	appLog = logrus.New()
)

func main() {
	app := cli.App("Endpoint Hitter", appDescription)

	appSystemCode := app.String(cli.StringOpt{
		Name:   "app-system-code",
		Value:  "endpoint-hitter",
		Desc:   "System Code of the application",
		EnvVar: "APP_SYSTEM_CODE",
	})

	appName := app.String(cli.StringOpt{
		Name:   "app-name",
		Value:  "Endpoint Hitter",
		Desc:   "Application name",
		EnvVar: "APP_NAME",
	})

	targetURL := app.String(cli.StringOpt{
		Name:   "target-url",
		Value:  "https://{env-domain}/__post-publication-combiner/{uuid}",
		Desc:   "URL address that the application intends to hit",
		EnvVar: "TARGET_URL",
	})

	methodType := app.String(cli.StringOpt{
		Name:   "method-type",
		Value:  "POST",
		Desc:   "GET, POST, PUT",
		EnvVar: "METHOD_TYPE",
	})

	authUser := app.String(cli.StringOpt{
		Name:   "auth-user",
		Desc:   "User required for authentication",
		EnvVar: "AUTH_USER",
	})

	authPassword := app.String(cli.StringOpt{
		Name:   "auth-password",
		Desc:   "Password required for authentication",
		EnvVar: "AUTH_PASSWORD",
	})

	throttle := app.Int(cli.IntOpt{
		Name:   "throttle",
		Value:  100,
		Desc:   "Number of parallel requests",
		EnvVar: "THROTTLE",
	})

	uuidFilePath := app.String(cli.StringOpt{
		Name:   "uuid-file-path",
		Value:  "uuids.txt",
		Desc:   "Relative path to the file containing all the input uuids.",
		EnvVar: "UUID_FILE_PATH",
	})

	appLog.Info("[Startup] Endpoint Hitter is starting")

	client = http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConnsPerHost:   50,
		},
	}

	app.Action = func() {
		appLog.Infof("System code: %s, App Name: %s", *appSystemCode, *appName)

		var uuids []string
		if file, err := os.Open(*uuidFilePath); err == nil {
			// make sure it gets closed
			defer file.Close()

			// create a new scanner and read the file line by line
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				el := scanner.Text()
				uuids = append(uuids, el)
			}
			// check for errors
			if err = scanner.Err(); err != nil {
				appLog.Fatal(err)
			}
		} else {
			appLog.Fatal(err)
		}

		successCh := make(chan struct{}, 1)
		successCounter := 0
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			for range successCh {
				successCounter++
			}
			wg.Done()
		}()
		start := time.Now()

		hitEndpoint(*targetURL, *methodType, *authUser, *authPassword, uuids, *throttle, successCh)

		elapsed := time.Since(start)
		wg.Wait()
		appLog.Printf("Import took %s, out of %v contents success count is: %v, success rate: %.2f%%", elapsed, len(uuids), successCounter, float64(successCounter)/float64(len(uuids))*100)
	}

	if err := app.Run(os.Args); err != nil {
		appLog.Fatalf("App could not start, error=[%s]\n", err)
		return
	}
}

func hitEndpoint(targetURL string, methodType string, authUser string, authPassword string, uuids []string, throttle int, successCh chan struct{}) {
	authKey := "Basic " + base64.StdEncoding.EncodeToString([]byte(authUser+":"+authPassword))

	count := 0
	limit := throttle

	for {
		if count == len(uuids) {
			close(successCh)
			break
		}

		if count+limit > len(uuids) {
			limit = len(uuids) - count
		}

		var wg sync.WaitGroup
		wg.Add(limit)

		for i := 0; i < limit; i++ {
			go func(uuid string) {
				defer wg.Done()
				url := strings.Replace(targetURL, "{uuid}", uuid, -1)
				retryCount := 0
				for {
					if retryCount == maxRetries {
						appLog.WithField("url", url).Errorf("Failed after %v retries", maxRetries)
						break
					}
					status, tid, err := executeHTTPRequest(url, methodType, authKey)
					if err == nil {
						successCh <- struct{}{}
						break
					}
					appLog.WithField("transaction_id", tid).WithField("url", url).WithField("status", status).WithField("retry", retryCount).Errorf("Error: %v", err.Error())
					if status != http.StatusGatewayTimeout && status != http.StatusServiceUnavailable {
						//permanent error
						break
					}
					time.Sleep(3 * time.Second)
					retryCount++
				}

			}(uuids[count+i])
		}
		wg.Wait()

		count = count + limit
	}
}

func executeHTTPRequest(urlStr string, methodType string, authKey string) (status int, transactionID string, err error) {
	req, err := http.NewRequest(methodType, urlStr, nil)

	transactionID = "tid_" + uniuri.NewLen(10) + "_endpoint-hitter"
	//Log continuously the transaction ids to see a some kind of status. Remove if not needed.
	req.Header.Add("X-Request-Id", transactionID)
	req.Header.Add("Authorization", authKey)

	if err != nil {
		return http.StatusInternalServerError, transactionID, fmt.Errorf("creating request returned error: %v", err)
	}

	resp, err := client.Do(req)

	if err != nil {
		return http.StatusInternalServerError, transactionID, fmt.Errorf("executing request returned error: %v", err)
	}

	defer cleanUp(resp)

	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, transactionID, fmt.Errorf("request returned a non-successfull response code")
	}

	_, err = ioutil.ReadAll(resp.Body)
	return http.StatusOK, transactionID, err
}

func cleanUp(resp *http.Response) {
	_, err := io.Copy(ioutil.Discard, resp.Body)
	if err != nil {
		appLog.Warningf("[%v]", err)
	}

	err = resp.Body.Close()
	if err != nil {
		appLog.Warningf("[%v]", err)
	}
}
