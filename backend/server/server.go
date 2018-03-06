package server

import (
	"github.com/iota-tangle-io/dtlg/backend/controllers"
	"github.com/iota-tangle-io/dtlg/backend/routers"
	"github.com/iota-tangle-io/dtlg/backend/utilities"
	"fmt"
	"github.com/facebookgo/inject"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"html/template"
	"io"
	"time"
	"context"
	"github.com/skratchdot/open-golang/open"
	"os"
	"path/filepath"
	"gopkg.in/inconshreveable/log15.v2"
)

type TemplateRendered struct {
	templates *template.Template
}

func (t *TemplateRendered) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

type Server struct {
	Config       *Configuration
	WebEngine    *echo.Echo
	ShutdownChan chan struct{}
	logger log15.Logger
}

func (server *Server) Start() {
	start := time.Now().UnixNano()
	server.ShutdownChan = make(chan struct{}, 1)

	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	exPath := filepath.Dir(ex)

	// load config
	configuration := LoadConfig()
	server.Config = configuration
	appConfig := server.Config.App
	httpConfig := server.Config.Net.HTTP

	// init logger
	utilities.Debug = appConfig.Verbose
	logger, err := utilities.GetLogger("app")
	if err != nil {
		panic(err)
	}
	server.logger = logger
	logger.Info("booting up app...")

	// init web server
	e := echo.New()
	server.WebEngine = e
	server.WebEngine.HideBanner = true
	if httpConfig.LogRequests {
		requestLogFile, err := os.Create(fmt.Sprintf("./logs/requests.log"))
		if err != nil {
			panic(err)
		}
		e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{Output: requestLogFile}))
		e.Logger.SetLevel(3)
	}

	// load html files
	e.Renderer = &TemplateRendered{
		templates: template.Must(template.ParseGlob(fmt.Sprintf("%s/*.html", exPath+httpConfig.Assets.HTML))),
	}

	// asset paths
	e.Static("/assets", exPath+httpConfig.Assets.Static)
	e.File("/favicon.ico", exPath+httpConfig.Assets.Favicon)

	// create controllers
	appCtrl := &controllers.AppCtrl{}
	spammerCtrl := &controllers.SpammerCtrl{}
	controllers := []controllers.Controller{
		appCtrl, spammerCtrl,
	}

	// create routers
	indexRouter := &routers.IndexRouter{}
	spammRouter := &routers.SpammerRouter{}
	spammRouter.ShutdownSignal = server.ShutdownChan
	rters := []routers.Router{indexRouter, spammRouter}

	// create injection graph for automatic dependency injection
	g := inject.Graph{}

	// add various objects to the graph
	if err = g.Provide(
		&inject.Object{Value: e},
		&inject.Object{Value: appConfig.Dev, Name: "dev"},
	); err != nil {
		panic(err)
	}

	// add controllers to graph
	for _, controller := range controllers {
		if err = g.Provide(&inject.Object{Value: controller}); err != nil {
			panic(err)
		}
	}

	// add routers to graph
	for _, router := range rters {
		if err = g.Provide(&inject.Object{Value: router}); err != nil {
			panic(err)
		}
	}

	// run dependency injection
	if err = g.Populate(); err != nil {
		panic(err)
	}

	// init controllers
	for _, controller := range controllers {
		if err = controller.Init(); err != nil {
			panic(err)
		}
	}
	logger.Info("initialised controllers")

	// init routers
	for _, router := range rters {
		router.Init()
	}
	logger.Info("initialised routers")

	// boot up server
	go e.Start(httpConfig.Address)

	if configuration.App.AutoOpenBrowser {
		open.Start("http://localhost:9090")
	}

	// finish
	delta := (time.Now().UnixNano() - start) / 1000000
	logger.Info(fmt.Sprintf("%s ready", configuration.App.Name), "startup", delta)

}

func (server *Server) Shutdown(timeout time.Duration) {
	server.logger.Info("shutting down DTLG")
	select {
	case <-time.After(timeout):
		ctx, _ := context.WithTimeout(context.Background(), timeout)
		server.WebEngine.Shutdown(ctx)
	}
}
