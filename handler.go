package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"github.com/dchest/uniuri"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"
)

type requestHandler struct {
	targetUrl    string
	methodType   string
	authUser     string
	authPassword string
	uuidFilePath string
	throttle     int
}

func (handler *requestHandler) getFile(writer http.ResponseWriter, request *http.Request) {
	defer request.Body.Close()

	var Buf bytes.Buffer

	file, header, err := request.FormFile("file")
	if err != nil {
		panic(err)
	}

	defer file.Close()

	name := strings.Split(header.Filename, ".")
	fmt.Printf("File name %s\n", name[0])

	io.Copy(&Buf, file)

	handler.processFile(&Buf)
}

func (handler *requestHandler) processFile(buffer *bytes.Buffer) {
	var uuids []string
	scanner := bufio.NewScanner(buffer)
	for scanner.Scan() {
		el := scanner.Text()
		uuids = append(uuids, el)
	}
	// check for errors
	if err := scanner.Err(); err != nil {
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

	handler.hitEndpoint(handler.targetUrl, handler.methodType, handler.authUser, handler.authPassword, uuids, handler.throttle, successCh)

	elapsed := time.Since(start)
	wg.Wait()
	appLog.Printf("Import took %s, out of %v contents success count is: %v, success rate: %.2f%%", elapsed, len(uuids), successCounter, float64(successCounter)/float64(len(uuids))*100)
}

func (handler *requestHandler) hitEndpoint(targetURL string, methodType string, authUser string, authPassword string, uuids []string, throttle int, successCh chan struct{}) {
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
