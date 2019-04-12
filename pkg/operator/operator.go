package operator

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	_ "github.com/prestodb/presto-go-client/presto"
	promapi "github.com/prometheus/client_golang/api"
	prom "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/transport"
	"k8s.io/client-go/util/workqueue"

	cbTypes "github.com/operator-framework/operator-metering/pkg/apis/metering/v1alpha1"
	"github.com/operator-framework/operator-metering/pkg/db"
	cbClientset "github.com/operator-framework/operator-metering/pkg/generated/clientset/versioned"
	factory "github.com/operator-framework/operator-metering/pkg/generated/informers/externalversions"
	listers "github.com/operator-framework/operator-metering/pkg/generated/listers/metering/v1alpha1"
	"github.com/operator-framework/operator-metering/pkg/hive"
	"github.com/operator-framework/operator-metering/pkg/operator/prestostore"
	"github.com/operator-framework/operator-metering/pkg/operator/reporting"
	_ "github.com/operator-framework/operator-metering/pkg/util/reflector/prometheus" // for prometheus metric registration
	_ "github.com/operator-framework/operator-metering/pkg/util/workqueue/prometheus" // for prometheus metric registration
)

const (
	connBackoff         = time.Second * 15
	maxConnRetries      = 3
	defaultResyncPeriod = time.Minute * 15
	prestoUsername      = "reporting-operator"

	DefaultPrometheusQueryInterval                       = time.Minute * 5  // Query Prometheus every 5 minutes
	DefaultPrometheusQueryStepSize                       = time.Minute      // Query data from Prometheus at a 60 second resolution (one data point per minute max)
	DefaultPrometheusQueryChunkSize                      = 5 * time.Minute  // the default value for how much data we will insert into Presto per Prometheus query.
	DefaultPrometheusDataSourceMaxQueryRangeDuration     = 10 * time.Minute // how much data we will query from Prometheus at once
	DefaultPrometheusDataSourceMaxBackfillImportDuration = 2 * time.Hour    // how far we will query for backlogged data.
)

type TLSConfig struct {
	UseTLS  bool
	TLSCert string
	TLSKey  string
}

func (cfg *TLSConfig) Valid() error {
	if cfg.UseTLS {
		if cfg.TLSCert == "" {
			return fmt.Errorf("Must set TLS certificate if TLS is enabled")
		}
		if cfg.TLSKey == "" {
			return fmt.Errorf("Must set TLS private key if TLS is enabled")
		}
	}
	return nil
}

type PrometheusConfig struct {
	Address         string
	SkipTLSVerify   bool
	BearerToken     string
	BearerTokenFile string
	CAFile          string
}

type Config struct {
	Hostname     string
	OwnNamespace string

	AllNamespaces    bool
	TargetNamespaces []string

	Kubeconfig string

	APIListen     string
	MetricsListen string
	PprofListen   string

	HiveHost   string
	PrestoHost string

	PrestoMaxQueryLength int

	DisablePromsum   bool
	EnableFinalizers bool

	LogDMLQueries bool
	LogDDLQueries bool

	PrometheusQueryConfig                         cbTypes.PrometheusQueryConfig
	PrometheusDataSourceMaxQueryRangeDuration     time.Duration
	PrometheusDataSourceMaxBackfillImportDuration time.Duration
	PrometheusDataSourceGlobalImportFromTime      *time.Time

	LeaderLeaseDuration time.Duration

	APITLSConfig     TLSConfig
	MetricsTLSConfig TLSConfig
	PrometheusConfig PrometheusConfig
}

type Reporting struct {
	cfg        Config
	kubeConfig *rest.Config

	meteringClient cbClientset.Interface
	kubeClient     corev1.CoreV1Interface

	informerFactory factory.SharedInformerFactory

	prestoTableLister           listers.PrestoTableLister
	hiveTableLister             listers.HiveTableLister
	reportDataSourceLister      listers.ReportDataSourceLister
	reportGenerationQueryLister listers.ReportGenerationQueryLister
	reportPrometheusQueryLister listers.ReportPrometheusQueryLister
	reportLister                listers.ReportLister
	storageLocationLister       listers.StorageLocationLister

	queueList                  []workqueue.RateLimitingInterface
	reportQueue                workqueue.RateLimitingInterface
	reportDataSourceQueue      workqueue.RateLimitingInterface
	reportGenerationQueryQueue workqueue.RateLimitingInterface
	prestoTableQueue           workqueue.RateLimitingInterface
	hiveTableQueue             workqueue.RateLimitingInterface
	storageLocationQueue       workqueue.RateLimitingInterface

	reportResultsRepo     prestostore.ReportResultsRepo
	prometheusMetricsRepo prestostore.PrometheusMetricsRepo
	reportGenerator       reporting.ReportGenerator

	prestoTableManager   reporting.PrestoTableManager
	hiveDatabaseManager  reporting.HiveDatabaseManager
	hiveTableManager     reporting.HiveTableManager
	hivePartitionManager reporting.HivePartitionManager

	testReadFromPrestoFunc func() bool

	promConn prom.API

	clock clock.Clock
	rand  *rand.Rand

	logger log.FieldLogger

	initializedMu sync.Mutex
	initialized   bool

	importersMu sync.Mutex
	importers   map[string]*prestostore.PrometheusImporter
}

func New(logger log.FieldLogger, cfg Config) (*Reporting, error) {
	if err := cfg.APITLSConfig.Valid(); err != nil {
		return nil, err
	}
	if err := cfg.MetricsTLSConfig.Valid(); err != nil {
		return nil, err
	}

	logger.Debugf("config: %s", spew.Sprintf("%+v", cfg))

	if cfg.AllNamespaces {
		logger.Infof("watching all namespaces for metering.openshift.io resources")
	} else {
		logger.Infof("watching namespaces %q for metering.openshift.io resources", cfg.TargetNamespaces)
	}

	configOverrides := &clientcmd.ConfigOverrides{}
	var clientConfig clientcmd.ClientConfig
	if cfg.Kubeconfig == "" {
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		clientConfig = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	} else {
		apiCfg, err := clientcmd.LoadFromFile(cfg.Kubeconfig)
		if err != nil {
			return nil, err
		}
		clientConfig = clientcmd.NewDefaultClientConfig(*apiCfg, configOverrides)
	}

	var err error
	kubeConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("Unable to get Kubernetes client config: %v", err)
	}

	logger.Debugf("setting up Kubernetes client...")
	kubeClient, err := corev1.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("Unable to create Kubernetes client: %v", err)
	}

	logger.Debugf("setting up Metering client...")
	meteringClient, err := cbClientset.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("Unable to create Metering client: %v", err)
	}

	var informerNamespace string
	if cfg.AllNamespaces {
		informerNamespace = metav1.NamespaceAll
	} else if len(cfg.TargetNamespaces) == 1 {
		informerNamespace = cfg.TargetNamespaces[0]
	} else if len(cfg.TargetNamespaces) > 1 && !cfg.AllNamespaces {
		return nil, fmt.Errorf("must set --all-namespaces if more than one namespace is passed to --target-namespaces")
	} else {
		informerNamespace = cfg.OwnNamespace
	}

	clock := clock.RealClock{}
	rand := rand.New(rand.NewSource(clock.Now().Unix()))

	op := newReportingOperator(logger, clock, rand, cfg, kubeConfig, kubeClient, meteringClient, informerNamespace)

	return op, nil
}

func newReportingOperator(
	logger log.FieldLogger,
	clock clock.Clock,
	rand *rand.Rand,
	cfg Config,
	kubeConfig *rest.Config,
	kubeClient corev1.CoreV1Interface,
	meteringClient cbClientset.Interface,
	informerNamespace string,
) *Reporting {

	informerFactory := factory.NewFilteredSharedInformerFactory(meteringClient, defaultResyncPeriod, informerNamespace, nil)

	prestoTableInformer := informerFactory.Metering().V1alpha1().PrestoTables()
	hiveTableInformer := informerFactory.Metering().V1alpha1().HiveTables()
	reportDataSourceInformer := informerFactory.Metering().V1alpha1().ReportDataSources()
	reportGenerationQueryInformer := informerFactory.Metering().V1alpha1().ReportGenerationQueries()
	reportPrometheusQueryInformer := informerFactory.Metering().V1alpha1().ReportPrometheusQueries()
	reportInformer := informerFactory.Metering().V1alpha1().Reports()
	storageLocationInformer := informerFactory.Metering().V1alpha1().StorageLocations()

	reportQueue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "reports")
	reportDataSourceQueue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "reportdatasources")
	reportGenerationQueryQueue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "reportgenerationqueries")
	prestoTableQueue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "prestotables")
	hiveTableQueue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "hivetables")
	storageLocationQueue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "storagelocation")

	queueList := []workqueue.RateLimitingInterface{
		reportQueue,
		reportDataSourceQueue,
		reportGenerationQueryQueue,
		prestoTableQueue,
		hiveTableQueue,
		storageLocationQueue,
	}

	op := &Reporting{
		logger:         logger,
		cfg:            cfg,
		kubeConfig:     kubeConfig,
		meteringClient: meteringClient,
		kubeClient:     kubeClient,

		informerFactory: informerFactory,

		prestoTableLister:           prestoTableInformer.Lister(),
		hiveTableLister:             hiveTableInformer.Lister(),
		reportDataSourceLister:      reportDataSourceInformer.Lister(),
		reportGenerationQueryLister: reportGenerationQueryInformer.Lister(),
		reportPrometheusQueryLister: reportPrometheusQueryInformer.Lister(),
		reportLister:                reportInformer.Lister(),
		storageLocationLister:       storageLocationInformer.Lister(),

		queueList:                  queueList,
		reportQueue:                reportQueue,
		reportDataSourceQueue:      reportDataSourceQueue,
		reportGenerationQueryQueue: reportGenerationQueryQueue,
		prestoTableQueue:           prestoTableQueue,
		hiveTableQueue:             hiveTableQueue,
		storageLocationQueue:       storageLocationQueue,

		rand:      rand,
		clock:     clock,
		importers: make(map[string]*prestostore.PrometheusImporter),
	}

	// all eventHandlers are wrapped in an
	// inTargetNamespaceResourceEventHandler which verifies the resources
	// passed to the eventHandler functions have a metadata.namespace contained
	// in the list of TargetNamespaces, and if so, runs the eventHandler func.
	// If TargetNamespaces is empty, it will no-op and return the original
	// eventHandler.
	reportInformer.Informer().AddEventHandler(newInTargetNamespaceEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    op.addReport,
		UpdateFunc: op.updateReport,
		DeleteFunc: op.deleteReport,
	}, op.cfg.TargetNamespaces))

	reportDataSourceInformer.Informer().AddEventHandler(newInTargetNamespaceEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    op.addReportDataSource,
		UpdateFunc: op.updateReportDataSource,
		DeleteFunc: op.deleteReportDataSource,
	}, op.cfg.TargetNamespaces))

	reportGenerationQueryInformer.Informer().AddEventHandler(newInTargetNamespaceEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    op.addReportGenerationQuery,
		UpdateFunc: op.updateReportGenerationQuery,
	}, op.cfg.TargetNamespaces))

	prestoTableInformer.Informer().AddEventHandler(newInTargetNamespaceEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    op.addPrestoTable,
		UpdateFunc: op.updatePrestoTable,
		DeleteFunc: op.deletePrestoTable,
	}, op.cfg.TargetNamespaces))

	hiveTableInformer.Informer().AddEventHandler(newInTargetNamespaceEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    op.addHiveTable,
		UpdateFunc: op.updateHiveTable,
		DeleteFunc: op.deleteHiveTable,
	}, op.cfg.TargetNamespaces))

	storageLocationInformer.Informer().AddEventHandler(newInTargetNamespaceEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    op.addStorageLocation,
		UpdateFunc: op.updateStorageLocation,
		DeleteFunc: op.deleteStorageLocation,
	}, op.cfg.TargetNamespaces))

	return op
}

func (op *Reporting) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	// buffered big enough to hold the errs of each server we start.
	srvErrChan := make(chan error, 3)

	op.logger.Info("starting Metering operator")

	promServer := &http.Server{
		Addr:    op.cfg.MetricsListen,
		Handler: promhttp.Handler(),
	}
	pprofServer := newPprofServer(op.cfg.PprofListen)

	// start these servers at the beginning some pprof and metrics are
	// available before the reporting operator is ready
	op.logger.Info("starting Prometheus metrics & pprof servers")
	wg.Add(2)
	go func() {
		defer wg.Done()
		var srvErr error
		if op.cfg.MetricsTLSConfig.UseTLS {
			op.logger.Infof("Prometheus metrics server listening with TLS on %s", op.cfg.MetricsListen)
			srvErr = promServer.ListenAndServeTLS(op.cfg.MetricsTLSConfig.TLSCert, op.cfg.MetricsTLSConfig.TLSKey)
		} else {
			op.logger.Infof("Prometheus metrics server listening on %s", op.cfg.MetricsListen)
			srvErr = promServer.ListenAndServe()
		}
		op.logger.WithError(srvErr).Info("Prometheus metrics server exited")
		srvErrChan <- fmt.Errorf("Prometheus metrics server error: %v", srvErr)
	}()
	go func() {
		defer wg.Done()
		op.logger.Infof("pprof server listening on %s", op.cfg.PprofListen)
		srvErr := pprofServer.ListenAndServe()
		op.logger.WithError(srvErr).Info("pprof server exited")
		srvErrChan <- fmt.Errorf("pprof server error: %v", srvErr)
	}()

	go op.informerFactory.Start(ctx.Done())

	op.logger.Infof("setting up DB clients")
	hiveQueryer := hive.NewReconnectingQueryer(ctx, op.logger, op.cfg.HiveHost, connBackoff, maxConnRetries)
	defer hiveQueryer.Close()

	connStr := fmt.Sprintf("http://%s@%s?catalog=hive&schema=default", prestoUsername, op.cfg.PrestoHost)
	prestoQueryer, err := sql.Open("presto", connStr)
	if err != nil {
		return err
	}
	defer prestoQueryer.Close()

	op.promConn, err = op.newPrometheusConnFromURL(op.cfg.PrometheusConfig.Address)
	if err != nil {
		return err
	}

	op.logger.Info("waiting for caches to sync")
	for t, synced := range op.informerFactory.WaitForCacheSync(ctx.Done()) {
		if !synced {
			return fmt.Errorf("cache for %s not synced in time", t)
		}
	}

	var prestoQueryBufferPool *sync.Pool
	if op.cfg.PrestoMaxQueryLength > 0 {
		bufferPool := prestostore.NewBufferPool(op.cfg.PrestoMaxQueryLength)
		prestoQueryBufferPool = &bufferPool
	}

	loggingDMLPrestoQueryer := db.NewLoggingQueryer(prestoQueryer, op.logger, op.cfg.LogDMLQueries)
	op.reportResultsRepo = prestostore.NewReportResultsRepo(loggingDMLPrestoQueryer)
	op.reportGenerator = reporting.NewReportGenerator(op.logger, op.reportResultsRepo)
	op.prometheusMetricsRepo = prestostore.NewPrometheusMetricsRepo(loggingDMLPrestoQueryer, prestoQueryBufferPool)

	loggingDDLPrestoQueryer := db.NewLoggingQueryer(prestoQueryer, op.logger, op.cfg.LogDDLQueries)
	loggingDDLHiveQueryer := db.NewLoggingQueryer(hiveQueryer, op.logger, op.cfg.LogDDLQueries)
	prestoTableManager := reporting.NewPrestoTableManager(loggingDDLPrestoQueryer)
	hiveManager := reporting.NewHiveManager(loggingDDLHiveQueryer)

	op.prestoTableManager = prestoTableManager
	op.hiveTableManager = hiveManager
	op.hiveDatabaseManager = hiveManager
	op.hivePartitionManager = hiveManager

	prestoHealthChecker := reporting.NewPrestoHealthChecker(op.logger, prestoQueryer, hiveManager, "", "operator_health_check")
	op.testReadFromPrestoFunc = func() bool {
		return prestoHealthChecker.TestReadFromPrestoSingleFlight()
	}

	op.logger.Infof("starting HTTP server")
	apiRouter := newRouter(
		op.logger, op.rand, op.prometheusMetricsRepo, op.reportResultsRepo, op.importPrometheusForTimeRange,
		op.reportLister, op.reportGenerationQueryLister, op.prestoTableLister,
	)
	apiRouter.HandleFunc("/ready", op.readinessHandler)
	apiRouter.HandleFunc("/healthy", op.readinessHandler)

	httpServer := &http.Server{
		Addr:    op.cfg.APIListen,
		Handler: apiRouter,
	}

	// start the HTTP API server
	wg.Add(1)
	go func() {
		defer wg.Done()
		var srvErr error
		if op.cfg.APITLSConfig.UseTLS {
			op.logger.Infof("HTTP API server listening with TLS on 127.0.0.1:8080")
			srvErr = httpServer.ListenAndServeTLS(op.cfg.APITLSConfig.TLSCert, op.cfg.APITLSConfig.TLSKey)
		} else {
			op.logger.Infof("HTTP API server listening on %s", op.cfg.APIListen)
			srvErr = httpServer.ListenAndServe()
		}
		op.logger.WithError(srvErr).Info("HTTP API server exited")
		srvErrChan <- fmt.Errorf("HTTP API server error: %v", srvErr)
	}()

	// Poll until we can read from presto
	op.logger.Info("testing ability to read to Presto")
	err = wait.PollUntil(time.Second*5, func() (bool, error) {
		return op.testReadFromPrestoFunc(), nil
	}, ctx.Done())
	if err != nil {
		return err
	}
	op.logger.Info("writes to Presto are succeeding")

	op.logger.Info("basic initialization completed")
	op.setInitialized()

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(op.logger.Infof)
	eventBroadcaster.StartRecordingToSink(&corev1.EventSinkImpl{Interface: op.kubeClient.Events(op.cfg.OwnNamespace)})
	eventRecorder := eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: op.cfg.Hostname})

	rl, err := resourcelock.New(resourcelock.ConfigMapsResourceLock,
		op.cfg.OwnNamespace, "reporting-operator-leader-lease", op.kubeClient,
		resourcelock.ResourceLockConfig{
			Identity:      op.cfg.Hostname,
			EventRecorder: eventRecorder,
		})
	if err != nil {
		return fmt.Errorf("error creating lock %v", err)
	}

	// We use a new context instead of the parent because we want shutdown to
	// be different than leader election loss and do not want shutdown to
	// trigger the leader election lost case.
	lostLeaderCtx, leaderCancel := context.WithCancel(context.Background())
	defer leaderCancel()

	leader, err := leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
		Lock:          rl,
		LeaseDuration: op.cfg.LeaderLeaseDuration,
		RenewDeadline: op.cfg.LeaderLeaseDuration / 2,
		RetryPeriod:   2 * time.Second,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				op.logger.Infof("became leader")
				op.logger.Info("starting Metering workers")
				op.startWorkers(wg, ctx)
				op.logger.Infof("Metering workers started, watching for reports...")
			},
			OnStoppedLeading: func() {
				op.logger.Warn("leader election lost")
				leaderCancel()
			},
		},
	})
	if err != nil {
		return fmt.Errorf("error creating leader elector: %v", err)
	}

	op.logger.Infof("starting leader election")
	go leader.Run(ctx)

	// wait for an shutdown signal to begin shutdown.
	// if we lose leadership or an error occurs from one of our server
	// processes exit immediately.
	select {
	case <-ctx.Done():
		op.logger.Info("got stop signal, shutting down Metering operator")
	case <-lostLeaderCtx.Done():
		op.logger.Warnf("lost leadership election, forcing shut down of Metering operator")
		return fmt.Errorf("lost leadership election")
	case err := <-srvErrChan:
		op.logger.WithError(err).Error("server process failed, shutting down Metering operator")
		return fmt.Errorf("server process failed, err: %v", err)
	}

	// if we stop being leader or get a shutdown signal, stop the workers
	op.logger.Infof("stopping workers and collectors")

	// stop our running http servers
	wg.Add(3)
	go func() {
		op.logger.Infof("stopping HTTP API server")
		err := httpServer.Shutdown(context.TODO())
		if err != nil {
			op.logger.WithError(err).Warnf("got an error shutting down HTTP API server")
		}
		wg.Done()
	}()
	go func() {
		op.logger.Infof("stopping Prometheus metrics server")
		err := promServer.Shutdown(context.TODO())
		if err != nil {
			op.logger.WithError(err).Warnf("got an error shutting down Prometheus metrics server")
		}
		wg.Done()
	}()
	go func() {
		op.logger.Infof("stopping pprof server")
		err := pprofServer.Shutdown(context.TODO())
		if err != nil {
			op.logger.WithError(err).Warnf("got an error shutting down pprof server")
		}
		wg.Done()
	}()

	// shutdown queues so that they get drained, and workers can begin their
	// shutdown
	go op.shutdownQueues()

	// wait for our workers to stop
	wg.Wait()
	op.logger.Info("Metering workers and collectors stopped")
	return nil
}

func (op *Reporting) newPrometheusConnFromURL(url string) (prom.API, error) {
	transportConfig := &transport.Config{}
	if op.cfg.PrometheusConfig.CAFile != "" {
		if _, err := os.Stat(op.cfg.PrometheusConfig.CAFile); err == nil {
			// Use the configured CA for communicating to Prometheus
			transportConfig.TLS.CAFile = op.cfg.PrometheusConfig.CAFile
			op.logger.Infof("using %s as CA for Prometheus", op.cfg.PrometheusConfig.CAFile)
		} else {
			return nil, err
		}
	} else {
		op.logger.Infof("using system CAs for Prometheus")
		transportConfig.TLS.CAData = nil
		transportConfig.TLS.CAFile = ""
	}

	if op.cfg.PrometheusConfig.SkipTLSVerify {
		transportConfig.TLS.Insecure = op.cfg.PrometheusConfig.SkipTLSVerify
		transportConfig.TLS.CAData = nil
		transportConfig.TLS.CAFile = ""
	}
	if op.cfg.PrometheusConfig.BearerToken != "" {
		transportConfig.BearerToken = op.cfg.PrometheusConfig.BearerToken
	}
	if op.cfg.PrometheusConfig.BearerTokenFile != "" {
		transportConfig.BearerTokenFile = op.cfg.PrometheusConfig.BearerTokenFile
	}

	roundTripper, err := transport.New(transportConfig)
	if err != nil {
		return nil, err
	}

	return op.newPrometheusConn(promapi.Config{
		Address:      url,
		RoundTripper: roundTripper,
	})
}

func (op *Reporting) startWorkers(wg sync.WaitGroup, ctx context.Context) {
	stopCh := ctx.Done()

	startWorker := func(threads int, workerFunc func(id int)) {
		for i := 0; i < threads; i++ {
			i := i

			wg.Add(1)
			go func() {
				workerFunc(i)
				wg.Done()
			}()
		}
	}

	startWorker(4, func(i int) {
		op.logger.Infof("starting StorageLocation worker #%d", i)
		wait.Until(op.runStorageLocationWorker, time.Second, stopCh)
		op.logger.Infof("StorageLocation worker #%d stopped", i)
	})

	startWorker(10, func(i int) {
		op.logger.Infof("starting HiveTable worker #%d", i)
		wait.Until(op.runHiveTableWorker, time.Second, stopCh)
		op.logger.Infof("HiveTable worker #%d stopped", i)
	})

	startWorker(10, func(i int) {
		op.logger.Infof("starting PrestoTable worker")
		wait.Until(op.runPrestoTableWorker, time.Second, stopCh)
		op.logger.Infof("PrestoTable worker stopped")
	})

	startWorker(8, func(i int) {
		op.logger.Infof("starting ReportDataSource worker #%d", i)
		wait.Until(op.runReportDataSourceWorker, time.Second, stopCh)
		op.logger.Infof("ReportDataSource worker #%d stopped", i)
	})

	startWorker(4, func(i int) {
		op.logger.Infof("starting ReportGenerationQuery worker #%d", i)
		wait.Until(op.runReportGenerationQueryWorker, time.Second, stopCh)
		op.logger.Infof("ReportGenerationQuery worker #%d stopped", i)
	})

	startWorker(6, func(i int) {
		op.logger.Infof("starting Report worker #%d", i)
		wait.Until(op.runReportWorker, time.Second, stopCh)
		op.logger.Infof("Report worker #%d stopped", i)
	})
}

func (op *Reporting) setInitialized() {
	op.initializedMu.Lock()
	op.initialized = true
	op.initializedMu.Unlock()
}

func (op *Reporting) isInitialized() bool {
	op.initializedMu.Lock()
	initialized := op.initialized
	op.initializedMu.Unlock()
	return initialized
}

func (op *Reporting) newPrometheusConn(promConfig promapi.Config) (prom.API, error) {
	client, err := promapi.NewClient(promConfig)
	if err != nil {
		return nil, fmt.Errorf("can't connect to prometheus: %v", err)
	}
	return prom.NewAPI(client), nil
}
