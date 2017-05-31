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
	"time"
	"sync"
)

const appDescription = "Small application that is able to hit in parallel a requested endpoint - llogging whether the request was successful."

var (
	client      http.Client
	endpointLog = logrus.New()
	appLog      = logrus.New()
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
		Value: 100,
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
	logFilePath := strings.Split(*uuidFilePath,".")[0]+".log"
	file, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY, 0666)
	if err == nil {
		endpointLog.Out = file
	} else {
		appLog.Info("Failed to log to file, using default stderr")
	}

	client = http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConnsPerHost:   50,
			TLSHandshakeTimeout:   3 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
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

		start := time.Now()

		hitEndpoint(*targetURL, *methodType, *authUser, *authPassword, uuids, *throttle)

		elapsed := time.Since(start)
		appLog.Printf("Import took %s ", elapsed)

	}
	err = app.Run(os.Args)
	if err != nil {
		appLog.Fatalf("App could not start, error=[%s]\n", err)
		return
	}
}

func hitEndpoint(targetURL string, methodType string, authUser string, authPassword string, uuids []string, throttle int) {
	authKey := "Basic " + base64.StdEncoding.EncodeToString([]byte(authUser+":"+authPassword))
	appLog.Info(authKey)

	count := 0
	limit := throttle

	for {
		if count == len(uuids) {
			break
		}

		if count + limit > len(uuids) {
			limit =  len(uuids) - count
		}

		var wg sync.WaitGroup
		wg.Add(limit)

		for i := 0; i < limit; i++ {
			go func(uuid string) {
				defer wg.Done()
				url := strings.Replace(targetURL, "{uuid}", uuid, -1)
				_, status, tid, _ := executeHTTPRequest(url, methodType, uuid, authKey)
				endpointLog.Infof("Content with uuid: %s for transaction %s ended with status code: %d", uuid, tid, status)
			}(uuids[count + i])
		}
		wg.Wait()

		count = count + limit
	}
}

func executeHTTPRequest(urlStr string, methodType string, uuid string, authKey string) (b []byte, status int, transactionID string, err error) {
	req, err := http.NewRequest(methodType, urlStr, nil)

	transactionID = "tid_" + uniuri.NewLen(10) + "_endpoint-hitter"
	//Log continuously the transaction ids to see a some kind of status. Remove if not needed.
	appLog.Info(transactionID)
	req.Header.Add("X-Request-Id", transactionID)
	req.Header.Add("Authorization", authKey)

	if err != nil {
		return nil, http.StatusInternalServerError, transactionID, fmt.Errorf("Error creating requests for url=%s, error=%v", urlStr, err)
	}

	resp, err := client.Do(req)

	if err != nil {
		return nil, resp.StatusCode, transactionID, fmt.Errorf("Error executing requests for url=%s, error=%v", urlStr, err)
	}

	defer cleanUp(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, transactionID, fmt.Errorf("Connecting to %s was not successful. Status: %d", urlStr, resp.StatusCode)
	}

	b, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, http.StatusOK, transactionID, fmt.Errorf("Could not parse payload from response for url=%s, error=%v", urlStr, err)
	}

	return b, http.StatusOK, transactionID, err
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
