package main

import (
	"github.com/Financial-Times/go-logger"
	"github.com/gorilla/mux"
	"github.com/jawher/mow.cli"
	"github.com/lytics/logrus"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
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

	appPort := app.String(cli.StringOpt{
		Name:   "app-port",
		Value:  "8080",
		Desc:   "Application port",
		EnvVar: "APP_PORT",
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
			MaxIdleConnsPerHost: 50,
		},
	}

	app.Action = func() {
		appLog.Infof("System code: %s, App Name: %s", *appSystemCode, *appName)

		routeRequests(*appPort, &requestHandler{*targetURL, *methodType, *authUser,
			*authPassword, *uuidFilePath, *throttle})
	}

	if err := app.Run(os.Args); err != nil {
		appLog.Fatalf("App could not start, error=[%s]\n", err)
		return
	}
}

func routeRequests(port string, requestHandler *requestHandler) {
	servicesRouter := mux.NewRouter()
	servicesRouter.HandleFunc("/file", requestHandler.getFile).Methods("POST")

	server := &http.Server{Addr: ":" + port, Handler: servicesRouter}

	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		if err := server.ListenAndServe(); err != nil {
			logger.Infof("HTTP server closing with message: %v", err)
		}
		wg.Done()
	}()

	waitForSignal()
	logger.Infof("[Shutdown] Endpoint Hitter is shutting down")

	if err := server.Close(); err != nil {
		logger.WithError(err).Error("Unable to stop http server")
	}

	wg.Wait()
}

func waitForSignal() {
	ch := make(chan os.Signal)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
}
