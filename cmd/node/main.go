package main

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/big"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	logger "github.com/ElrondNetwork/elrond-go-logger"
	"github.com/ElrondNetwork/elrond-go-logger/redirects"
	"github.com/ElrondNetwork/elrond-go/cmd/node/factory"
	"github.com/ElrondNetwork/elrond-go/cmd/node/metrics"
	"github.com/ElrondNetwork/elrond-go/config"
	"github.com/ElrondNetwork/elrond-go/consensus"
	"github.com/ElrondNetwork/elrond-go/consensus/round"
	"github.com/ElrondNetwork/elrond-go/core"
	"github.com/ElrondNetwork/elrond-go/core/accumulator"
	"github.com/ElrondNetwork/elrond-go/core/alarm"
	"github.com/ElrondNetwork/elrond-go/core/check"
	"github.com/ElrondNetwork/elrond-go/core/closing"
	"github.com/ElrondNetwork/elrond-go/core/indexer"
	"github.com/ElrondNetwork/elrond-go/core/statistics"
	"github.com/ElrondNetwork/elrond-go/core/watchdog"
	"github.com/ElrondNetwork/elrond-go/crypto"
	"github.com/ElrondNetwork/elrond-go/data"
	"github.com/ElrondNetwork/elrond-go/data/endProcess"
	"github.com/ElrondNetwork/elrond-go/data/state"
	"github.com/ElrondNetwork/elrond-go/data/typeConverters"
	"github.com/ElrondNetwork/elrond-go/dataRetriever"
	"github.com/ElrondNetwork/elrond-go/epochStart"
	"github.com/ElrondNetwork/elrond-go/epochStart/bootstrap"
	"github.com/ElrondNetwork/elrond-go/epochStart/notifier"
	"github.com/ElrondNetwork/elrond-go/facade"
	mainFactory "github.com/ElrondNetwork/elrond-go/factory"
	"github.com/ElrondNetwork/elrond-go/genesis/parsing"
	"github.com/ElrondNetwork/elrond-go/hashing"
	"github.com/ElrondNetwork/elrond-go/health"
	"github.com/ElrondNetwork/elrond-go/marshal"
	"github.com/ElrondNetwork/elrond-go/node"
	"github.com/ElrondNetwork/elrond-go/node/external"
	"github.com/ElrondNetwork/elrond-go/node/nodeDebugFactory"
	"github.com/ElrondNetwork/elrond-go/ntp"
	"github.com/ElrondNetwork/elrond-go/process"
	"github.com/ElrondNetwork/elrond-go/process/coordinator"
	"github.com/ElrondNetwork/elrond-go/process/economics"
	processFactory "github.com/ElrondNetwork/elrond-go/process/factory"
	"github.com/ElrondNetwork/elrond-go/process/factory/metachain"
	"github.com/ElrondNetwork/elrond-go/process/factory/shard"
	"github.com/ElrondNetwork/elrond-go/process/interceptors"
	"github.com/ElrondNetwork/elrond-go/process/rating"
	"github.com/ElrondNetwork/elrond-go/process/rating/peerHonesty"
	"github.com/ElrondNetwork/elrond-go/process/smartContract"
	"github.com/ElrondNetwork/elrond-go/process/smartContract/builtInFunctions"
	"github.com/ElrondNetwork/elrond-go/process/smartContract/hooks"
	"github.com/ElrondNetwork/elrond-go/process/throttle/antiflood/blackList"
	"github.com/ElrondNetwork/elrond-go/process/transaction"
	"github.com/ElrondNetwork/elrond-go/sharding"
	"github.com/ElrondNetwork/elrond-go/storage"
	storageFactory "github.com/ElrondNetwork/elrond-go/storage/factory"
	"github.com/ElrondNetwork/elrond-go/storage/lrucache"
	"github.com/ElrondNetwork/elrond-go/storage/storageUnit"
	"github.com/ElrondNetwork/elrond-go/storage/timecache"
	"github.com/ElrondNetwork/elrond-go/update"
	exportFactory "github.com/ElrondNetwork/elrond-go/update/factory"
	"github.com/ElrondNetwork/elrond-go/update/trigger"
	"github.com/ElrondNetwork/elrond-go/vm"
	"github.com/ElrondNetwork/elrond-vm-common/parsers"
	"github.com/denisbrodbeck/machineid"
	"github.com/google/gops/agent"
	"github.com/urfave/cli"
)

const (
	notSetDestinationShardID = "disabled"
	maxTimeToClose           = 10 * time.Second
	maxMachineIDLen          = 10
)

var (
	nodeHelpTemplate = `NAME:
   {{.Name}} - {{.Usage}}
USAGE:
   {{.HelpName}} {{if .VisibleFlags}}[global options]{{end}}
   {{if len .Authors}}
AUTHOR:
   {{range .Authors}}{{ . }}{{end}}
   {{end}}{{if .Commands}}
GLOBAL OPTIONS:
   {{range .VisibleFlags}}{{.}}
   {{end}}
VERSION:
   {{.Version}}
   {{end}}
`
	rm *statistics.ResourceMonitor

	// appVersion should be populated at build time using ldflags
	// Usage examples:
	// linux/mac:
	//            go build -i -v -ldflags="-X main.appVersion=$(git describe --tags --long --dirty)"
	// windows:
	//            for /f %i in ('git describe --tags --long --dirty') do set VERS=%i
	//            go build -i -v -ldflags="-X main.appVersion=%VERS%"
	appVersion = core.UnVersionedAppString
)

type configs struct {
	generalConfig                    *config.Config
	apiRoutesConfig                  *config.ApiRoutesConfig
	economicsConfig                  *config.EconomicsConfig
	systemSCConfig                   *config.SystemSmartContractsConfig
	ratingsConfig                    *config.RatingsConfig
	preferencesConfig                *config.Preferences
	externalConfig                   *config.ExternalConfig
	p2pConfig                        *config.P2PConfig
	configurationFileName            string
	configurationEconomicsFileName   string
	configurationRatingsFileName     string
	configurationPreferencesFileName string
	p2pConfigurationFileName         string
}

func main() {
	_ = logger.SetDisplayByteSlice(logger.ToHexShort)
	log := logger.GetOrCreate("main")

	app := cli.NewApp()
	cli.AppHelpTemplate = nodeHelpTemplate
	app.Name = "Elrond Node CLI App"
	machineID, err := machineid.ProtectedID(app.Name)
	if err != nil {
		log.Warn("error fetching machine id", "error", err)
		machineID = "unknown"
	}
	if len(machineID) > maxMachineIDLen {
		machineID = machineID[:maxMachineIDLen]
	}

	app.Version = fmt.Sprintf("%s/%s/%s-%s/%s", appVersion, runtime.Version(), runtime.GOOS, runtime.GOARCH, machineID)
	app.Usage = "This is the entry point for starting a new Elrond node - the app will start after the genesis timestamp"
	app.Flags = getFlags()
	app.Authors = []cli.Author{
		{
			Name:  "The Elrond Team",
			Email: "contact@elrond.com",
		},
	}

	app.Action = func(c *cli.Context) error {
		return startNode(c, log, app.Version)
	}

	err = app.Run(os.Args)
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
}

func startNode(ctx *cli.Context, log logger.Logger, version string) error {
	log.Trace("startNode called")

	workingDir := getWorkingDir(ctx, log)
	logFile, err := updateLogger(workingDir, ctx, log)
	if err != nil {
		return err
	}

	enableGopsIfNeeded(ctx, log)

	log.Info("starting node", "version", version, "pid", os.Getpid())

	var cfgs *configs
	cfgs, err = readConfigs(log, ctx)
	if err != nil {
		return err
	}

	log.Trace("creating core components")

	coreArgs := mainFactory.CoreComponentsHandlerArgs{
		Config:           *cfgs.generalConfig,
		WorkingDirectory: workingDir,
	}
	managedCoreComponents, err := mainFactory.NewManagedCoreComponents(coreArgs)
	if err != nil {
		return err
	}

	err = managedCoreComponents.Create()
	if err != nil {
		return err
	}

	//TODO when refactoring main, maybe initialize economics data before this line
	totalSupply, ok := big.NewInt(0).SetString(cfgs.economicsConfig.GlobalSettings.GenesisTotalSupply, 10)
	if !ok {
		return fmt.Errorf("can not parse total suply from economics.toml, %s is not a valid value",
			cfgs.economicsConfig.GlobalSettings.GenesisTotalSupply)
	}

	log.Debug("config", "file", ctx.GlobalString(genesisFile.Name))

	genesisNodesConfig, err := sharding.NewNodesSetup(
		ctx.GlobalString(nodesFile.Name),
		managedCoreComponents.AddressPubKeyConverter(),
		managedCoreComponents.ValidatorPubKeyConverter(),
	)
	if err != nil {
		return err
	}
	log.Debug("config", "file", ctx.GlobalString(nodesFile.Name))

	syncer := ntp.NewSyncTime(cfgs.generalConfig.NTPConfig, nil)
	syncer.StartSyncingTime()

	log.Debug("NTP average clock offset", "value", syncer.ClockOffset())

	if ctx.IsSet(startInEpoch.Name) {
		log.Debug("start in epoch is enabled")
		cfgs.generalConfig.GeneralSettings.StartInEpochEnabled = ctx.GlobalBool(startInEpoch.Name)
		if cfgs.generalConfig.GeneralSettings.StartInEpochEnabled {
			delayedStartInterval := 2 * time.Second
			time.Sleep(delayedStartInterval)
		}
	}

	//TODO: The next 5 lines should be deleted when we are done testing from a precalculated (not hard coded) timestamp
	if genesisNodesConfig.StartTime == 0 {
		time.Sleep(1000 * time.Millisecond)
		ntpTime := syncer.CurrentTime()
		genesisNodesConfig.StartTime = (ntpTime.Unix()/60 + 1) * 60
	}

	startTime := time.Unix(genesisNodesConfig.StartTime, 0)

	log.Info("start time",
		"formatted", startTime.Format("Mon Jan 2 15:04:05 MST 2006"),
		"seconds", startTime.Unix())
	validatorKeyPemFileName := ctx.GlobalString(validatorKeyPemFile.Name)

	log.Trace("creating crypto components")

	cryptoComponentsHandlerArgs := mainFactory.CryptoComponentsHandlerArgs{
		ValidatorKeyPemFileName:              validatorKeyPemFileName,
		SkIndex:                              ctx.GlobalInt(validatorKeyIndex.Name),
		Config:                               *cfgs.generalConfig,
		CoreComponentsHolder:                 managedCoreComponents,
		ActivateBLSPubKeyMessageVerification: cfgs.economicsConfig.ValidatorSettings.ActivateBLSPubKeyMessageVerification,
	}

	managedCryptoComponents, err := mainFactory.NewManagedCryptoComponents(cryptoComponentsHandlerArgs)
	if err != nil {
		return err
	}

	err = managedCryptoComponents.Create()
	if err != nil {
		return err
	}

	log.Debug("block sign pubkey", "value", managedCryptoComponents.PublicKeyString())

	if ctx.IsSet(destinationShardAsObserver.Name) {
		cfgs.preferencesConfig.Preferences.DestinationShardAsObserver = ctx.GlobalString(destinationShardAsObserver.Name)
	}

	if ctx.IsSet(nodeDisplayName.Name) {
		cfgs.preferencesConfig.Preferences.NodeDisplayName = ctx.GlobalString(nodeDisplayName.Name)
	}

	if ctx.IsSet(identityFlagName.Name) {
		cfgs.preferencesConfig.Preferences.Identity = ctx.GlobalString(identityFlagName.Name)
	}

	err = cleanupStorageIfNecessary(workingDir, ctx, log)
	if err != nil {
		return err
	}

	genesisShardCoordinator, nodeType, err := createShardCoordinator(
		genesisNodesConfig,
		managedCryptoComponents.PublicKey(),
		cfgs.preferencesConfig.Preferences,
		log,
	)
	if err != nil {
		return err
	}
	var shardId = core.GetShardIDString(genesisShardCoordinator.SelfId())

	accountsParser, err := parsing.NewAccountsParser(
		ctx.GlobalString(genesisFile.Name),
		totalSupply,
		managedCoreComponents.AddressPubKeyConverter(),
		managedCryptoComponents.TxSignKeyGen(),
	)
	if err != nil {
		return err
	}

	smartContractParser, err := parsing.NewSmartContractsParser(
		ctx.GlobalString(smartContractsFile.Name),
		managedCoreComponents.AddressPubKeyConverter(),
		managedCryptoComponents.TxSignKeyGen(),
	)
	if err != nil {
		return err
	}

	healthService := health.NewHealthService(cfgs.generalConfig.Health, workingDir)
	if ctx.IsSet(useHealthService.Name) {
		healthService.Start()
	}

	chanCreateViews := make(chan struct{}, 1)
	chanLogRewrite := make(chan struct{}, 1)
	handlersArgs, err := factory.NewStatusHandlersFactoryArgs(
		useLogView.Name,
		ctx,
		managedCoreComponents.InternalMarshalizer(),
		managedCoreComponents.Uint64ByteSliceConverter(),
		chanCreateViews,
		chanLogRewrite,
		logFile,
	)
	if err != nil {
		return err
	}

	statusHandlersInfo, err := factory.CreateStatusHandlers(handlersArgs)
	if err != nil {
		return err
	}

	err = managedCoreComponents.SetStatusHandler(statusHandlersInfo.StatusHandler)
	if err != nil {
		return err
	}

	log.Trace("creating network components")
	args := mainFactory.NetworkComponentsFactoryArgs{
		P2pConfig:     *cfgs.p2pConfig,
		MainConfig:    *cfgs.generalConfig,
		StatusHandler: managedCoreComponents.StatusHandler(),
		Marshalizer:   managedCoreComponents.InternalMarshalizer(),
	}

	managedNetworkComponents, err := mainFactory.NewManagedNetworkComponents(args)
	if err != nil {
		return err
	}
	err = managedNetworkComponents.Create()
	if err != nil {
		return err
	}

	err = managedNetworkComponents.NetworkMessenger().Bootstrap()
	if err != nil {
		return err
	}
	log.Info(fmt.Sprintf("waiting %d seconds for network discovery...", core.SecondsToWaitForP2PBootstrap))
	time.Sleep(core.SecondsToWaitForP2PBootstrap * time.Second)

	log.Trace("creating economics data components")
	economicsData, err := economics.NewEconomicsData(cfgs.economicsConfig)
	if err != nil {
		return err
	}

	log.Trace("creating ratings data components")

	ratingDataArgs := rating.RatingsDataArg{
		Config:                   *cfgs.ratingsConfig,
		ShardConsensusSize:       genesisNodesConfig.ConsensusGroupSize,
		MetaConsensusSize:        genesisNodesConfig.MetaChainConsensusGroupSize,
		ShardMinNodes:            genesisNodesConfig.MinNodesPerShard,
		MetaMinNodes:             genesisNodesConfig.MetaChainMinNodes,
		RoundDurationMiliseconds: genesisNodesConfig.RoundDuration,
	}
	ratingsData, err := rating.NewRatingsData(ratingDataArgs)
	if err != nil {
		return err
	}

	rater, err := rating.NewBlockSigningRater(ratingsData)
	if err != nil {
		return err
	}

	nodesShuffler := sharding.NewHashValidatorsShuffler(
		genesisNodesConfig.MinNodesPerShard,
		genesisNodesConfig.MetaChainMinNodes,
		genesisNodesConfig.Hysteresis,
		genesisNodesConfig.Adaptivity,
		cfgs.generalConfig.EpochStartConfig.ShuffleBetweenShards,
	)

	destShardIdAsObserver, err := processDestinationShardAsObserver(cfgs.preferencesConfig.Preferences)
	if err != nil {
		return err
	}

	startRound := int64(0)
	if cfgs.generalConfig.Hardfork.AfterHardFork {
		startRound = int64(cfgs.generalConfig.Hardfork.StartRound)
	}
	rounder, err := round.NewRound(
		time.Unix(genesisNodesConfig.StartTime, 0),
		syncer.CurrentTime(),
		time.Millisecond*time.Duration(genesisNodesConfig.RoundDuration),
		syncer,
		startRound,
	)
	if err != nil {
		return err
	}

	importStartHandler, err := trigger.NewImportStartHandler(filepath.Join(workingDir, core.DefaultDBPath), appVersion)
	if err != nil {
		return err
	}

	bootstrapDataProvider, err := storageFactory.NewBootstrapDataProvider(managedCoreComponents.InternalMarshalizer())
	if err != nil {
		return err
	}

	latestStorageDataProvider, err := factory.CreateLatestStorageDataProvider(
		bootstrapDataProvider,
		managedCoreComponents.InternalMarshalizer(),
		managedCoreComponents.Hasher(),
		*cfgs.generalConfig,
		managedCoreComponents.ChainID(),
		workingDir,
		core.DefaultDBPath,
		core.DefaultEpochString,
		core.DefaultShardString,
	)
	if err != nil {
		return err
	}

	unitOpener, err := factory.CreateUnitOpener(
		bootstrapDataProvider,
		latestStorageDataProvider,
		managedCoreComponents.InternalMarshalizer(),
		*cfgs.generalConfig,
		managedCoreComponents.ChainID(),
		workingDir,
		core.DefaultDBPath,
		core.DefaultEpochString,
		core.DefaultShardString,
	)
	if err != nil {
		return err
	}

	epochStartBootstrapArgs := bootstrap.ArgsEpochStartBootstrap{
		CoreComponentsHolder:       managedCoreComponents,
		CryptoComponentsHolder:     managedCryptoComponents,
		Messenger:                  managedNetworkComponents.NetworkMessenger(),
		GeneralConfig:              *cfgs.generalConfig,
		EconomicsData:              economicsData,
		GenesisNodesConfig:         genesisNodesConfig,
		GenesisShardCoordinator:    genesisShardCoordinator,
		StorageUnitOpener:          unitOpener,
		Rater:                      rater,
		DestinationShardAsObserver: destShardIdAsObserver,
		NodeShuffler:               nodesShuffler,
		Rounder:                    rounder,
		LatestStorageDataProvider:  latestStorageDataProvider,
		ArgumentsParser:            smartContract.NewArgumentParser(),
		StatusHandler:              managedCoreComponents.StatusHandler(),
	}

	bootstrapper, err := bootstrap.NewEpochStartBootstrap(epochStartBootstrapArgs)
	if err != nil {
		log.Error("could not create bootstrap", "err", err)
		return err
	}

	bootstrapParameters, err := bootstrapper.Bootstrap()
	if err != nil {
		log.Error("bootstrap return error", "error", err)
		return err
	}

	log.Info("bootstrap parameters", "shardId", bootstrapParameters.SelfShardId, "epoch", bootstrapParameters.Epoch, "numShards", bootstrapParameters.NumOfShards)

	shardCoordinator, err := sharding.NewMultiShardCoordinator(bootstrapParameters.NumOfShards, bootstrapParameters.SelfShardId)
	if err != nil {
		return err
	}

	currentEpoch := bootstrapParameters.Epoch
	storerEpoch := currentEpoch
	if !cfgs.generalConfig.StoragePruning.Enabled {
		// TODO: refactor this as when the pruning storer is disabled, the default directory path is Epoch_0
		// and it should be Epoch_ALL or something similar
		storerEpoch = 0
	}

	var shardIdString = core.GetShardIDString(shardCoordinator.SelfId())
	logger.SetCorrelationShard(shardIdString)

	log.Trace("initializing stats file")
	err = initStatsFileMonitor(
		cfgs.generalConfig,
		managedCryptoComponents.PublicKeyString(),
		log,
		workingDir,
		managedCoreComponents.PathHandler(),
		shardId)
	if err != nil {
		return err
	}

	log.Trace("creating state components")
	stateArgs := mainFactory.StateComponentsFactoryArgs{
		Config:           *cfgs.generalConfig,
		ShardCoordinator: shardCoordinator,
		Core:             managedCoreComponents,
	}
	managedStateComponents, err := mainFactory.NewManagedStateComponents(stateArgs)
	if err != nil {
		return err
	}

	err = managedStateComponents.Create()
	if err != nil {
		return err
	}

	trieContainer, trieStorageManager := bootstrapper.GetTriesComponents()
	err = managedStateComponents.SetTriesContainer(trieContainer)
	if err != nil {
		return err
	}
	err = managedStateComponents.SetTriesStorageManagers(trieStorageManager)
	if err != nil {
		return err
	}

	log.Trace("creating data components")
	epochStartNotifier := notifier.NewEpochStartSubscriptionHandler()

	dataArgs := mainFactory.DataComponentsHandlerArgs{
		Config:             *cfgs.generalConfig,
		EconomicsData:      economicsData,
		ShardCoordinator:   shardCoordinator,
		Core:               managedCoreComponents,
		EpochStartNotifier: epochStartNotifier,
		CurrentEpoch:       storerEpoch,
	}

	managedDataComponents, err := mainFactory.NewManagedDataComponents(dataArgs)
	if err != nil {
		return err
	}
	err = managedDataComponents.Create()
	if err != nil {
		return err
	}

	healthService.RegisterComponent(managedDataComponents.Datapool().Transactions())
	healthService.RegisterComponent(managedDataComponents.Datapool().UnsignedTransactions())
	healthService.RegisterComponent(managedDataComponents.Datapool().RewardTransactions())

	log.Trace("initializing metrics")
	err = metrics.InitMetrics(
		managedCoreComponents.StatusHandler(),
		managedCryptoComponents.PublicKeyString(),
		nodeType,
		shardCoordinator,
		genesisNodesConfig,
		version,
		cfgs.economicsConfig,
		cfgs.generalConfig.EpochStartConfig.RoundsPerEpoch,
		managedCoreComponents.MinTransactionVersion(),
	)
	if err != nil {
		return err
	}

	chanLogRewrite <- struct{}{}
	chanCreateViews <- struct{}{}

	err = statusHandlersInfo.UpdateStorerAndMetricsForPersistentHandler(
		managedDataComponents.StorageService().GetStorer(dataRetriever.StatusMetricsUnit),
	)
	if err != nil {
		return err
	}

	log.Trace("creating nodes coordinator")
	if ctx.IsSet(keepOldEpochsData.Name) {
		cfgs.generalConfig.StoragePruning.CleanOldEpochsData = !ctx.GlobalBool(keepOldEpochsData.Name)
	}
	if ctx.IsSet(numEpochsToSave.Name) {
		cfgs.generalConfig.StoragePruning.NumEpochsToKeep = ctx.GlobalUint64(numEpochsToSave.Name)
	}
	if ctx.IsSet(numActivePersisters.Name) {
		cfgs.generalConfig.StoragePruning.NumActivePersisters = ctx.GlobalUint64(numActivePersisters.Name)
	}
	log.Info("Bootstrap", "epoch", bootstrapParameters.Epoch)
	if bootstrapParameters.NodesConfig != nil {
		log.Info("the epoch from nodesConfig is", "epoch", bootstrapParameters.NodesConfig.CurrentEpoch)
	}
	chanStopNodeProcess := make(chan endProcess.ArgEndProcess, 1)
	nodesCoordinator, nodeShufflerOut, err := createNodesCoordinator(
		genesisNodesConfig,
		cfgs.preferencesConfig.Preferences,
		epochStartNotifier,
		managedCryptoComponents.PublicKey(),
		managedCoreComponents.InternalMarshalizer(),
		managedCoreComponents.Hasher(),
		rater,
		managedDataComponents.StorageService().GetStorer(dataRetriever.BootstrapUnit),
		nodesShuffler,
		cfgs.generalConfig.EpochStartConfig,
		shardCoordinator.SelfId(),
		chanStopNodeProcess,
		bootstrapParameters,
		currentEpoch,
	)
	if err != nil {
		return err
	}

	metrics.SaveStringMetric(managedCoreComponents.StatusHandler(), core.MetricNodeDisplayName, cfgs.preferencesConfig.Preferences.NodeDisplayName)
	metrics.SaveStringMetric(managedCoreComponents.StatusHandler(), core.MetricChainId, managedCoreComponents.ChainID())
	metrics.SaveUint64Metric(managedCoreComponents.StatusHandler(), core.MetricGasPerDataByte, economicsData.GasPerDataByte())
	metrics.SaveUint64Metric(managedCoreComponents.StatusHandler(), core.MetricMinGasPrice, economicsData.MinGasPrice())
	metrics.SaveUint64Metric(managedCoreComponents.StatusHandler(), core.MetricMinGasLimit, economicsData.MinGasLimit())

	sessionInfoFileOutput := fmt.Sprintf("%s:%s\n%s:%s\n%s:%v\n%s:%s\n%s:%v\n",
		"PkBlockSign", managedCryptoComponents.PublicKeyString(),
		"ShardId", shardId,
		"TotalShards", shardCoordinator.NumberOfShards(),
		"AppVersion", version,
		"GenesisTimeStamp", startTime.Unix(),
	)

	sessionInfoFileOutput += fmt.Sprintf("\nStarted with parameters:\n")
	for _, flag := range ctx.App.Flags {
		flagValue := fmt.Sprintf("%v", ctx.GlobalGeneric(flag.GetName()))
		if flagValue != "" {
			sessionInfoFileOutput += fmt.Sprintf("%s = %v\n", flag.GetName(), flagValue)
		}
	}

	statsFolder := filepath.Join(workingDir, core.DefaultStatsPath)
	copyConfigToStatsFolder(
		statsFolder,
		[]string{
			cfgs.configurationFileName,
			cfgs.configurationEconomicsFileName,
			cfgs.configurationRatingsFileName,
			cfgs.configurationPreferencesFileName,
			cfgs.p2pConfigurationFileName,
			cfgs.configurationFileName,
			ctx.GlobalString(genesisFile.Name),
			ctx.GlobalString(nodesFile.Name),
		})

	statsFile := filepath.Join(statsFolder, "session.info")
	err = ioutil.WriteFile(statsFile, []byte(sessionInfoFileOutput), os.ModePerm)
	log.LogIfError(err)

	//TODO: remove this in the future and add just a log debug
	computedRatingsData := filepath.Join(statsFolder, "ratings.info")
	computedRatingsDataStr := createStringFromRatingsData(ratingsData)
	err = ioutil.WriteFile(computedRatingsData, []byte(computedRatingsDataStr), os.ModePerm)
	log.LogIfError(err)

	gasScheduleConfigurationFileName := ctx.GlobalString(gasScheduleConfigurationFile.Name)
	gasSchedule, err := core.LoadGasScheduleConfig(gasScheduleConfigurationFileName)
	if err != nil {
		return err
	}

	log.Trace("creating time cache for requested items components")
	requestedItemsHandler := timecache.NewTimeCache(time.Duration(uint64(time.Millisecond) * genesisNodesConfig.RoundDuration))

	whiteListCache, err := storageUnit.NewCache(storageFactory.GetCacherFromConfig(cfgs.generalConfig.WhiteListPool))
	if err != nil {
		return err
	}
	whiteListRequest, err := interceptors.NewWhiteListDataVerifier(whiteListCache)
	if err != nil {
		return err
	}

	whiteListerVerifiedTxs, err := createWhiteListerVerifiedTxs(cfgs.generalConfig)
	if err != nil {
		return err
	}

	log.Trace("starting status pooling components")
	statArgs := mainFactory.StatusComponentsFactoryArgs{
		Config:             *cfgs.generalConfig,
		ExternalConfig:     *cfgs.externalConfig,
		RoundDurationSec:   genesisNodesConfig.RoundDuration / 1000,
		ElasticOptions:     &indexer.Options{TxIndexingEnabled: ctx.GlobalBoolT(enableTxIndexing.Name)},
		ShardCoordinator:   shardCoordinator,
		NodesCoordinator:   nodesCoordinator,
		EpochStartNotifier: epochStartNotifier,
		StatusUtils:        statusHandlersInfo,
		CoreComponents:     managedCoreComponents,
		DataComponents:     managedDataComponents,
		NetworkComponents:  managedNetworkComponents,
	}
	managedStatusComponents, err := mainFactory.NewManagedStatusComponents(statArgs)
	if err != nil {
		return err
	}
	err = managedStatusComponents.Create()
	if err != nil {
		return err
	}

	log.Trace("creating process components")
	coreServiceContainer, _ := managedStatusComponents.ServiceContainer()

	processArgs := mainFactory.ProcessComponentsFactoryArgs{
		CoreFactoryArgs:           (*mainFactory.CoreComponentsFactoryArgs)(&coreArgs),
		AccountsParser:            accountsParser,
		SmartContractParser:       smartContractParser,
		EconomicsData:             economicsData,
		NodesConfig:               genesisNodesConfig,
		GasSchedule:               gasSchedule,
		Rounder:                   rounder,
		ShardCoordinator:          shardCoordinator,
		NodesCoordinator:          nodesCoordinator,
		Data:                      managedDataComponents,
		CoreData:                  managedCoreComponents,
		Crypto:                    managedCryptoComponents,
		State:                     managedStateComponents,
		Network:                   managedNetworkComponents,
		CoreServiceContainer:      coreServiceContainer,
		RequestedItemsHandler:     requestedItemsHandler,
		WhiteListHandler:          whiteListRequest,
		WhiteListerVerifiedTxs:    whiteListerVerifiedTxs,
		EpochStartNotifier:        epochStartNotifier,
		EpochStart:                &cfgs.generalConfig.EpochStartConfig,
		Rater:                     rater,
		RatingsData:               ratingsData,
		StartEpochNum:             currentEpoch,
		SizeCheckDelta:            cfgs.generalConfig.Marshalizer.SizeCheckDelta,
		StateCheckpointModulus:    cfgs.generalConfig.StateTriesConfig.CheckpointRoundsModulus,
		MaxComputableRounds:       cfgs.generalConfig.GeneralSettings.MaxComputableRounds,
		NumConcurrentResolverJobs: cfgs.generalConfig.Antiflood.NumConcurrentResolverJobs,
		MinSizeInBytes:            cfgs.generalConfig.BlockSizeThrottleConfig.MinSizeInBytes,
		MaxSizeInBytes:            cfgs.generalConfig.BlockSizeThrottleConfig.MaxSizeInBytes,
		MaxRating:                 cfgs.ratingsConfig.General.MaxRating,
		ValidatorPubkeyConverter:  managedCoreComponents.ValidatorPubKeyConverter(),
		SystemSCConfig:            cfgs.systemSCConfig,
		Version:                   version,
		ImportStartHandler:        importStartHandler,
		WorkingDir:                workingDir,
	}

	managedProcessComponents, err := mainFactory.NewManagedProcessComponents(processArgs)
	if err != nil {
		return err
	}
	err = managedProcessComponents.Create()
	if err != nil {
		return err
	}

	managedStatusComponents.SetForkDetector(managedProcessComponents.ForkDetector())
	err = managedStatusComponents.StartPolling()

	hardForkTrigger, err := createHardForkTrigger(
		cfgs.generalConfig,
		shardCoordinator,
		nodesCoordinator,
		managedCoreComponents,
		managedStateComponents,
		managedDataComponents,
		managedCryptoComponents,
		managedProcessComponents,
		managedNetworkComponents,
		whiteListRequest,
		whiteListerVerifiedTxs,
		chanStopNodeProcess,
		epochStartNotifier,
		importStartHandler,
		workingDir,
	)
	if err != nil {
		return err
	}

	err = hardForkTrigger.AddCloser(nodeShufflerOut)
	if err != nil {
		return fmt.Errorf("%w when adding nodeShufflerOut in hardForkTrigger", err)
	}

	var elasticIndexer indexer.Indexer
	if !check.IfNil(coreServiceContainer) && !check.IfNil(coreServiceContainer.Indexer()) {
		elasticIndexer = coreServiceContainer.Indexer()
		elasticIndexer.SetTxLogsProcessor(managedProcessComponents.TxLogsProcessor())
		managedProcessComponents.TxLogsProcessor().EnableLogToBeSavedInCache()
	}

	log.Trace("creating node structure")
	currentNode, err := createNode(
		cfgs.generalConfig,
		*cfgs.ratingsConfig,
		cfgs.preferencesConfig,
		genesisNodesConfig,
		economicsData,
		syncer,
		managedCryptoComponents.BlockSignKeyGen(),
		managedCryptoComponents.PrivateKey(),
		managedCryptoComponents.PublicKey(),
		shardCoordinator,
		nodesCoordinator,
		managedCoreComponents,
		managedStateComponents,
		managedDataComponents,
		managedCryptoComponents,
		managedProcessComponents,
		managedNetworkComponents,
		ctx.GlobalUint64(bootstrapRoundIndex.Name),
		version,
		elasticIndexer,
		requestedItemsHandler,
		epochStartNotifier,
		whiteListRequest,
		whiteListerVerifiedTxs,
		chanStopNodeProcess,
		hardForkTrigger,
	)
	if err != nil {
		return err
	}

	log.Trace("creating software checker structure")
	softwareVersionChecker, err := factory.CreateSoftwareVersionChecker(
		managedCoreComponents.StatusHandler(),
		cfgs.generalConfig.SoftwareVersionConfig,
	)
	if err != nil {
		log.Debug("nil software version checker", "error", err.Error())
	} else {
		softwareVersionChecker.StartCheckSoftwareVersion()
	}

	if shardCoordinator.SelfId() == core.MetachainShardId {
		log.Trace("activating nodesCoordinator's validators indexing")
		indexValidatorsListIfNeeded(
			elasticIndexer,
			nodesCoordinator,
			managedProcessComponents.EpochStartTrigger().Epoch(),
			log,
		)
	}

	log.Trace("creating api resolver structure")
	apiResolver, err := createApiResolver(
		cfgs.generalConfig,
		managedStateComponents.AccountsAdapter(),
		managedStateComponents.PeerAccounts(),
		managedCoreComponents.AddressPubKeyConverter(),
		managedDataComponents.StorageService(),
		managedDataComponents.Blockchain(),
		managedCoreComponents.InternalMarshalizer(),
		managedCoreComponents.Hasher(),
		managedCoreComponents.Uint64ByteSliceConverter(),
		shardCoordinator,
		statusHandlersInfo.StatusMetrics,
		gasSchedule,
		economicsData,
		managedCryptoComponents.MessageSignVerifier(),
		genesisNodesConfig,
		cfgs.systemSCConfig,
	)
	if err != nil {
		return err
	}

	log.Trace("creating elrond node facade")
	restAPIServerDebugMode := ctx.GlobalBool(restApiDebug.Name)

	argNodeFacade := facade.ArgNodeFacade{
		Node:                   currentNode,
		ApiResolver:            apiResolver,
		RestAPIServerDebugMode: restAPIServerDebugMode,
		WsAntifloodConfig:      cfgs.generalConfig.Antiflood.WebServer,
		FacadeConfig: config.FacadeConfig{
			RestApiInterface: ctx.GlobalString(restApiInterface.Name),
			PprofEnabled:     ctx.GlobalBool(profileMode.Name),
		},
		ApiRoutesConfig: *cfgs.apiRoutesConfig,
	}

	ef, err := facade.NewNodeFacade(argNodeFacade)
	if err != nil {
		return fmt.Errorf("%w while creating NodeFacade", err)
	}

	ef.SetSyncer(syncer)
	ef.SetTpsBenchmark(managedStatusComponents.TpsBenchmark())

	log.Trace("starting background services")
	ef.StartBackgroundServices()

	log.Debug("starting node...")
	err = ef.StartNode()
	if err != nil {
		log.Error("starting node failed", "epoch", currentEpoch, "error", err.Error())
		return err
	}

	log.Info("application is now running")
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	var sig endProcess.ArgEndProcess
	select {
	case <-sigs:
		log.Info("terminating at user's signal...")
	case sig = <-chanStopNodeProcess:
		log.Info("terminating at internal stop signal", "reason", sig.Reason, "description", sig.Description)
	}

	chanCloseComponents := make(chan struct{})
	go func() {
		closeAllComponents(log, healthService, managedDataComponents, managedStateComponents, managedNetworkComponents, chanCloseComponents)
	}()

	select {
	case <-chanCloseComponents:
	case <-time.After(maxTimeToClose):
		log.Warn("force closing the node", "error", "closeAllComponents did not finished on time")
	}

	log.Debug("closing node")

	return nil
}

func closeAllComponents(
	log logger.Logger,
	healthService io.Closer,
	dataComponents mainFactory.DataComponentsHolder,
	stateComponents mainFactory.StateComponentsHolder,
	networkComponents mainFactory.NetworkComponentsHolder,
	chanCloseComponents chan struct{},
) {
	log.Debug("closing health service...")
	err := healthService.Close()
	log.LogIfError(err)

	log.Debug("closing all store units....")
	err = dataComponents.StorageService().CloseAll()
	log.LogIfError(err)

	dataTries := stateComponents.TriesContainer().GetAll()
	for _, trie := range dataTries {
		err = trie.ClosePersister()
		log.LogIfError(err)
	}

	if rm != nil {
		err = rm.Close()
		log.LogIfError(err)
	}

	log.Debug("calling close on the network messenger instance...")
	err = networkComponents.NetworkMessenger().Close()
	log.LogIfError(err)

	chanCloseComponents <- struct{}{}
}

func createStringFromRatingsData(ratingsData *rating.RatingsData) string {
	metaChainStepHandler := ratingsData.MetaChainRatingsStepHandler()
	shardChainHandler := ratingsData.ShardChainRatingsStepHandler()
	computedRatingsDataStr := fmt.Sprintf(
		"meta:\n"+
			"ProposerIncrease=%v\n"+
			"ProposerDecrease=%v\n"+
			"ValidatorIncrease=%v\n"+
			"ValidatorDecrease=%v\n\n"+
			"shard:\n"+
			"ProposerIncrease=%v\n"+
			"ProposerDecrease=%v\n"+
			"ValidatorIncrease=%v\n"+
			"ValidatorDecrease=%v",
		metaChainStepHandler.ProposerIncreaseRatingStep(),
		metaChainStepHandler.ProposerDecreaseRatingStep(),
		metaChainStepHandler.ValidatorIncreaseRatingStep(),
		metaChainStepHandler.ValidatorDecreaseRatingStep(),
		shardChainHandler.ProposerIncreaseRatingStep(),
		shardChainHandler.ProposerDecreaseRatingStep(),
		shardChainHandler.ValidatorIncreaseRatingStep(),
		shardChainHandler.ValidatorDecreaseRatingStep(),
	)
	return computedRatingsDataStr
}

func cleanupStorageIfNecessary(workingDir string, ctx *cli.Context, log logger.Logger) error {
	storageCleanupFlagValue := ctx.GlobalBool(storageCleanup.Name)
	if storageCleanupFlagValue {
		dbPath := filepath.Join(
			workingDir,
			core.DefaultDBPath)
		log.Trace("cleaning storage", "path", dbPath)
		err := os.RemoveAll(dbPath)
		if err != nil {
			return err
		}
	}
	return nil
}

func copyConfigToStatsFolder(statsFolder string, configs []string) {
	for _, configFile := range configs {
		copySingleFile(statsFolder, configFile)
	}
}

func copySingleFile(folder string, configFile string) {
	fileName := filepath.Base(configFile)

	source, err := core.OpenFile(configFile)
	if err != nil {
		return
	}
	defer func() {
		err = source.Close()
		if err != nil {
			fmt.Println(fmt.Sprintf("Could not close %s", source.Name()))
		}
	}()

	destPath := filepath.Join(folder, fileName)
	destination, err := os.Create(destPath)
	if err != nil {
		return
	}
	defer func() {
		err = destination.Close()
		if err != nil {
			fmt.Println(fmt.Sprintf("Could not close %s", source.Name()))
		}
	}()

	_, err = io.Copy(destination, source)
	if err != nil {
		fmt.Println(fmt.Sprintf("Could not copy %s", source.Name()))
	}
}

func getWorkingDir(ctx *cli.Context, log logger.Logger) string {
	var workingDir string
	var err error
	if ctx.IsSet(workingDirectory.Name) {
		workingDir = ctx.GlobalString(workingDirectory.Name)
	} else {
		workingDir, err = os.Getwd()
		if err != nil {
			log.LogIfError(err)
			workingDir = ""
		}
	}
	log.Trace("working directory", "path", workingDir)

	return workingDir
}

func prepareLogFile(workingDir string) (*os.File, error) {
	logDirectory := filepath.Join(workingDir, core.DefaultLogsPath)
	fileForLog, err := core.CreateFile("elrond-go", logDirectory, "log")
	if err != nil {
		return nil, err
	}

	//we need this function as to close file.Close() when the code panics and the defer func associated
	//with the file pointer in the main func will never be reached
	runtime.SetFinalizer(fileForLog, func(f *os.File) {
		_ = f.Close()
	})

	err = redirects.RedirectStderr(fileForLog)
	if err != nil {
		return nil, err
	}

	err = logger.AddLogObserver(fileForLog, &logger.PlainFormatter{})
	if err != nil {
		return nil, fmt.Errorf("%w adding file log observer", err)
	}

	return fileForLog, nil
}

func indexValidatorsListIfNeeded(
	elasticIndexer indexer.Indexer,
	coordinator sharding.NodesCoordinator,
	epoch uint32,
	log logger.Logger,

) {
	if check.IfNil(elasticIndexer) {
		return
	}

	validatorsPubKeys, err := coordinator.GetAllEligibleValidatorsPublicKeys(epoch)
	if err != nil {
		log.Warn("GetAllEligibleValidatorPublicKeys for epoch 0 failed", "error", err)
	}

	if len(validatorsPubKeys) > 0 {
		go elasticIndexer.SaveValidatorsPubKeys(validatorsPubKeys, epoch)
	}
}

func enableGopsIfNeeded(ctx *cli.Context, log logger.Logger) {
	var gopsEnabled bool
	if ctx.IsSet(gopsEn.Name) {
		gopsEnabled = ctx.GlobalBool(gopsEn.Name)
	}

	if gopsEnabled {
		if err := agent.Listen(agent.Options{}); err != nil {
			log.Error("failure to init gops", "error", err.Error())
		}
	}

	log.Trace("gops", "enabled", gopsEnabled)
}

func loadMainConfig(filepath string) (*config.Config, error) {
	cfg := &config.Config{}
	err := core.LoadTomlFile(cfg, filepath)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func loadApiConfig(filepath string) (*config.ApiRoutesConfig, error) {
	cfg := &config.ApiRoutesConfig{}
	err := core.LoadTomlFile(cfg, filepath)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func loadEconomicsConfig(filepath string) (*config.EconomicsConfig, error) {
	cfg := &config.EconomicsConfig{}
	err := core.LoadTomlFile(cfg, filepath)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func loadSystemSmartContractsConfig(filepath string) (*config.SystemSmartContractsConfig, error) {
	cfg := &config.SystemSmartContractsConfig{}
	err := core.LoadTomlFile(cfg, filepath)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func loadRatingsConfig(filepath string) (*config.RatingsConfig, error) {
	cfg := &config.RatingsConfig{}
	err := core.LoadTomlFile(cfg, filepath)
	if err != nil {
		return &config.RatingsConfig{}, err
	}

	return cfg, nil
}

func loadPreferencesConfig(filepath string) (*config.Preferences, error) {
	cfg := &config.Preferences{}
	err := core.LoadTomlFile(cfg, filepath)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func loadExternalConfig(filepath string) (*config.ExternalConfig, error) {
	cfg := &config.ExternalConfig{}
	err := core.LoadTomlFile(cfg, filepath)
	if err != nil {
		return nil, fmt.Errorf("cannot load external config: %w", err)
	}

	return cfg, nil
}

func getShardIdFromNodePubKey(pubKey crypto.PublicKey, nodesConfig *sharding.NodesSetup) (uint32, error) {
	if pubKey == nil {
		return 0, errors.New("nil public key")
	}

	publicKey, err := pubKey.ToByteArray()
	if err != nil {
		return 0, err
	}

	selfShardId, err := nodesConfig.GetShardIDForPubKey(publicKey)
	if err != nil {
		return 0, err
	}

	return selfShardId, err
}

func createShardCoordinator(
	nodesConfig *sharding.NodesSetup,
	pubKey crypto.PublicKey,
	prefsConfig config.PreferencesConfig,
	log logger.Logger,
) (sharding.Coordinator, core.NodeType, error) {

	selfShardId, err := getShardIdFromNodePubKey(pubKey, nodesConfig)
	nodeType := core.NodeTypeValidator
	if err == sharding.ErrPublicKeyNotFoundInGenesis {
		nodeType = core.NodeTypeObserver
		log.Info("starting as observer node")

		selfShardId, err = processDestinationShardAsObserver(prefsConfig)
		if err != nil {
			return nil, "", err
		}
		if selfShardId == core.DisabledShardIDAsObserver {
			selfShardId = uint32(0)
		}
	}
	if err != nil {
		return nil, "", err
	}

	var shardName string
	if selfShardId == core.MetachainShardId {
		shardName = core.MetachainShardName
	} else {
		shardName = fmt.Sprintf("%d", selfShardId)
	}
	log.Info("shard info", "started in shard", shardName)

	shardCoordinator, err := sharding.NewMultiShardCoordinator(nodesConfig.NumberOfShards(), selfShardId)
	if err != nil {
		return nil, "", err
	}

	return shardCoordinator, nodeType, nil
}

func createNodesCoordinator(
	nodesConfig *sharding.NodesSetup,
	prefsConfig config.PreferencesConfig,
	epochStartNotifier epochStart.RegistrationHandler,
	pubKey crypto.PublicKey,
	marshalizer marshal.Marshalizer,
	hasher hashing.Hasher,
	ratingAndListIndexHandler sharding.PeerAccountListAndRatingHandler,
	bootStorer storage.Storer,
	nodeShuffler sharding.NodesShuffler,
	epochConfig config.EpochStartConfig,
	currentShardID uint32,
	chanStopNodeProcess chan endProcess.ArgEndProcess,
	bootstrapParameters bootstrap.Parameters,
	startEpoch uint32,
) (sharding.NodesCoordinator, update.Closer, error) {
	shardIDAsObserver, err := processDestinationShardAsObserver(prefsConfig)
	if err != nil {
		return nil, nil, err
	}
	if shardIDAsObserver == core.DisabledShardIDAsObserver {
		shardIDAsObserver = uint32(0)
	}

	nbShards := nodesConfig.NumberOfShards()
	shardConsensusGroupSize := int(nodesConfig.ConsensusGroupSize)
	metaConsensusGroupSize := int(nodesConfig.MetaChainConsensusGroupSize)
	eligibleNodesInfo, waitingNodesInfo := nodesConfig.InitialNodesInfo()

	eligibleValidators, errEligibleValidators := sharding.NodesInfoToValidators(eligibleNodesInfo)
	if errEligibleValidators != nil {
		return nil, nil, errEligibleValidators
	}

	waitingValidators, errWaitingValidators := sharding.NodesInfoToValidators(waitingNodesInfo)
	if errWaitingValidators != nil {
		return nil, nil, errWaitingValidators
	}

	currentEpoch := startEpoch
	if bootstrapParameters.NodesConfig != nil {
		nodeRegistry := bootstrapParameters.NodesConfig
		currentEpoch = bootstrapParameters.Epoch
		eligibles := nodeRegistry.EpochsConfig[fmt.Sprintf("%d", currentEpoch)].EligibleValidators
		eligibleValidators, err = sharding.SerializableValidatorsToValidators(eligibles)
		if err != nil {
			return nil, nil, err
		}

		waitings := nodeRegistry.EpochsConfig[fmt.Sprintf("%d", currentEpoch)].WaitingValidators
		waitingValidators, err = sharding.SerializableValidatorsToValidators(waitings)
		if err != nil {
			return nil, nil, err
		}
	}

	pubKeyBytes, err := pubKey.ToByteArray()
	if err != nil {
		return nil, nil, err
	}

	consensusGroupCache, err := lrucache.NewCache(25000)
	if err != nil {
		return nil, nil, err
	}

	thresholdEpochDuration := epochConfig.ShuffledOutRestartThreshold
	if !(thresholdEpochDuration >= 0.0 && thresholdEpochDuration <= 1.0) {
		return nil, nil, fmt.Errorf("invalid threshold for shuffled out handler")
	}
	maxDurationBeforeStopProcess := int64(nodesConfig.RoundDuration) * epochConfig.RoundsPerEpoch
	maxDurationBeforeStopProcess = int64(thresholdEpochDuration * float64(maxDurationBeforeStopProcess))
	maxDurationInterval := time.Millisecond * time.Duration(maxDurationBeforeStopProcess)
	minDurationInterval := maxDurationInterval / 2
	//waiting interval will be [maxDuration/2 and maxDuration]

	nodeShufflerOut, err := closing.NewShuffleOutCloser(
		minDurationInterval,
		maxDurationInterval,
		chanStopNodeProcess,
	)
	if err != nil {
		return nil, nil, err
	}
	shuffledOutHandler, err := sharding.NewShuffledOutTrigger(pubKeyBytes, currentShardID, nodeShufflerOut.EndOfProcessingHandler)
	if err != nil {
		return nil, nil, err
	}

	argumentsNodesCoordinator := sharding.ArgNodesCoordinator{
		ShardConsensusGroupSize: shardConsensusGroupSize,
		MetaConsensusGroupSize:  metaConsensusGroupSize,
		Marshalizer:             marshalizer,
		Hasher:                  hasher,
		Shuffler:                nodeShuffler,
		EpochStartNotifier:      epochStartNotifier,
		BootStorer:              bootStorer,
		ShardIDAsObserver:       shardIDAsObserver,
		NbShards:                nbShards,
		EligibleNodes:           eligibleValidators,
		WaitingNodes:            waitingValidators,
		SelfPublicKey:           pubKeyBytes,
		ConsensusGroupCache:     consensusGroupCache,
		ShuffledOutHandler:      shuffledOutHandler,
		Epoch:                   currentEpoch,
		StartEpoch:              startEpoch,
	}

	baseNodesCoordinator, err := sharding.NewIndexHashedNodesCoordinator(argumentsNodesCoordinator)
	if err != nil {
		return nil, nil, err
	}

	nodesCoordinator, err := sharding.NewIndexHashedNodesCoordinatorWithRater(baseNodesCoordinator, ratingAndListIndexHandler)
	if err != nil {
		return nil, nil, err
	}

	return nodesCoordinator, nodeShufflerOut, nil
}

func processDestinationShardAsObserver(prefsConfig config.PreferencesConfig) (uint32, error) {
	destShard := strings.ToLower(prefsConfig.DestinationShardAsObserver)
	if len(destShard) == 0 {
		return 0, errors.New("option DestinationShardAsObserver is not set in prefs.toml")
	}

	if destShard == notSetDestinationShardID {
		return core.DisabledShardIDAsObserver, nil
	}

	if destShard == core.MetachainShardName {
		return core.MetachainShardId, nil
	}

	val, err := strconv.ParseUint(destShard, 10, 32)
	if err != nil {
		return 0, errors.New("error parsing DestinationShardAsObserver option: " + err.Error())
	}

	return uint32(val), err
}

func getConsensusGroupSize(nodesConfig *sharding.NodesSetup, shardCoordinator sharding.Coordinator) (uint32, error) {
	if shardCoordinator.SelfId() == core.MetachainShardId {
		return nodesConfig.MetaChainConsensusGroupSize, nil
	}
	if shardCoordinator.SelfId() < shardCoordinator.NumberOfShards() {
		return nodesConfig.ConsensusGroupSize, nil
	}

	return 0, state.ErrUnknownShardId
}

func createHardForkTrigger(
	config *config.Config,
	shardCoordinator sharding.Coordinator,
	nodesCoordinator sharding.NodesCoordinator,
	coreData mainFactory.CoreComponentsHolder,
	stateComponents mainFactory.StateComponentsHolder,
	data mainFactory.DataComponentsHolder,
	crypto mainFactory.CryptoComponentsHolder,
	process mainFactory.ProcessComponentsHolder,
	network mainFactory.NetworkComponentsHolder,
	whiteListRequest process.WhiteListHandler,
	whiteListerVerifiedTxs process.WhiteListHandler,
	chanStopNodeProcess chan endProcess.ArgEndProcess,
	epochNotifier factory.EpochStartNotifier,
	importStartHandler update.ImportStartHandler,
	workingDir string,
) (node.HardforkTrigger, error) {

	selfPubKeyBytes := crypto.PublicKeyBytes()
	triggerPubKeyBytes, err := coreData.ValidatorPubKeyConverter().Decode(config.Hardfork.PublicKeyToListenFrom)
	if err != nil {
		return nil, fmt.Errorf("%w while decoding HardforkConfig.PublicKeyToListenFrom", err)
	}

	accountsDBs := make(map[state.AccountsDbIdentifier]state.AccountsAdapter)
	accountsDBs[state.UserAccountsState] = stateComponents.AccountsAdapter()
	accountsDBs[state.PeerAccountsState] = stateComponents.PeerAccounts()
	hardForkConfig := config.Hardfork
	exportFolder := filepath.Join(workingDir, hardForkConfig.ImportFolder)
	argsExporter := exportFactory.ArgsExporter{
		CoreComponents:           coreData,
		CryptoComponents:         crypto,
		HeaderValidator:          process.HeaderConstructionValidator(),
		DataPool:                 data.Datapool(),
		StorageService:           data.StorageService(),
		RequestHandler:           process.RequestHandler(),
		ShardCoordinator:         shardCoordinator,
		Messenger:                network.NetworkMessenger(),
		ActiveAccountsDBs:        accountsDBs,
		ExistingResolvers:        process.ResolversFinder(),
		ExportFolder:             exportFolder,
		ExportTriesStorageConfig: hardForkConfig.ExportTriesStorageConfig,
		ExportStateStorageConfig: hardForkConfig.ExportStateStorageConfig,
		ExportStateKeysConfig:    hardForkConfig.ExportKeysStorageConfig,
		WhiteListHandler:         whiteListRequest,
		WhiteListerVerifiedTxs:   whiteListerVerifiedTxs,
		InterceptorsContainer:    process.InterceptorsContainer(),
		NodesCoordinator:         nodesCoordinator,
		HeaderSigVerifier:        process.HeaderSigVerifier(),
		HeaderIntegrityVerifier:  process.HeaderIntegrityVerifier(),
		MaxTrieLevelInMemory:     config.StateTriesConfig.MaxStateTrieLevelInMemory,
		InputAntifloodHandler:    network.InputAntiFloodHandler(),
		OutputAntifloodHandler:   network.OutputAntiFloodHandler(),
		ValidityAttester:         process.BlockTracker(),
		Rounder:                  process.Rounder(),
	}
	hardForkExportFactory, err := exportFactory.NewExportHandlerFactory(argsExporter)
	if err != nil {
		return nil, err
	}

	atArgumentParser := smartContract.NewArgumentParser()
	argTrigger := trigger.ArgHardforkTrigger{
		TriggerPubKeyBytes:        triggerPubKeyBytes,
		SelfPubKeyBytes:           selfPubKeyBytes,
		Enabled:                   config.Hardfork.EnableTrigger,
		EnabledAuthenticated:      config.Hardfork.EnableTriggerFromP2P,
		ArgumentParser:            atArgumentParser,
		EpochProvider:             process.EpochStartTrigger(),
		ExportFactoryHandler:      hardForkExportFactory,
		ChanStopNodeProcess:       chanStopNodeProcess,
		EpochConfirmedNotifier:    epochNotifier,
		CloseAfterExportInMinutes: config.Hardfork.CloseAfterExportInMinutes,
		ImportStartHandler:        importStartHandler,
	}
	hardforkTrigger, err := trigger.NewTrigger(argTrigger)
	if err != nil {
		return nil, err
	}

	return hardforkTrigger, nil
}

func createNode(
	config *config.Config,
	ratingConfig config.RatingsConfig,
	preferencesConfig *config.Preferences,
	nodesConfig *sharding.NodesSetup,
	economicsData process.FeeHandler,
	syncer ntp.SyncTimer,
	keyGen crypto.KeyGenerator,
	privKey crypto.PrivateKey,
	pubKey crypto.PublicKey,
	shardCoordinator sharding.Coordinator,
	nodesCoordinator sharding.NodesCoordinator,
	coreData mainFactory.CoreComponentsHolder,
	stateComponents mainFactory.StateComponentsHolder,
	data mainFactory.DataComponentsHolder,
	crypto mainFactory.CryptoComponentsHolder,
	process mainFactory.ProcessComponentsHolder,
	network mainFactory.NetworkComponentsHolder,
	bootstrapRoundIndex uint64,
	version string,
	indexer indexer.Indexer,
	requestedItemsHandler dataRetriever.RequestedItemsHandler,
	epochStartRegistrationHandler epochStart.RegistrationHandler,
	whiteListRequest process.WhiteListHandler,
	whiteListerVerifiedTxs process.WhiteListHandler,
	chanStopNodeProcess chan endProcess.ArgEndProcess,
	hardForkTrigger node.HardforkTrigger,
) (*node.Node, error) {
	var err error
	var consensusGroupSize uint32
	consensusGroupSize, err = getConsensusGroupSize(nodesConfig, shardCoordinator)
	if err != nil {
		return nil, err
	}

	var txAccumulator node.Accumulator
	txAccumulatorConfig := config.Antiflood.TxAccumulator
	txAccumulator, err = accumulator.NewTimeAccumulator(
		time.Duration(txAccumulatorConfig.MaxAllowedTimeInMilliseconds)*time.Millisecond,
		time.Duration(txAccumulatorConfig.MaxDeviationTimeInMilliseconds)*time.Millisecond,
	)
	if err != nil {
		return nil, err
	}

	PrepareOpenTopics(network.InputAntiFloodHandler(), shardCoordinator)

	alarmScheduler := alarm.NewAlarmScheduler()
	watchdogTimer, err := watchdog.NewWatchdog(alarmScheduler, chanStopNodeProcess)
	if err != nil {
		return nil, err
	}

	peerDenialEvaluator, err := blackList.NewPeerDenialEvaluator(
		network.PeerBlackListHandler(),
		network.PubKeyCacher(),
		process.PeerShardMapper(),
	)
	if err != nil {
		return nil, err
	}

	err = network.NetworkMessenger().SetPeerDenialEvaluator(peerDenialEvaluator)
	if err != nil {
		return nil, err
	}

	peerHonestyHandler, err := createPeerHonestyHandler(config, ratingConfig, network.PubKeyCacher())
	if err != nil {
		return nil, err
	}

	var nd *node.Node
	nd, err = node.NewNode(
		node.WithMessenger(network.NetworkMessenger()),
		node.WithHasher(coreData.Hasher()),
		node.WithInternalMarshalizer(coreData.InternalMarshalizer(), config.Marshalizer.SizeCheckDelta),
		node.WithVmMarshalizer(coreData.VmMarshalizer()),
		node.WithTxSignMarshalizer(coreData.TxMarshalizer()),
		node.WithTxFeeHandler(economicsData),
		node.WithInitialNodesPubKeys(nodesConfig.InitialNodesPubKeys()),
		node.WithAddressPubkeyConverter(coreData.AddressPubKeyConverter()),
		node.WithValidatorPubkeyConverter(coreData.ValidatorPubKeyConverter()),
		node.WithAccountsAdapter(stateComponents.AccountsAdapter()),
		node.WithBlockChain(data.Blockchain()),
		node.WithDataStore(data.StorageService()),
		node.WithRoundDuration(nodesConfig.RoundDuration),
		node.WithConsensusGroupSize(int(consensusGroupSize)),
		node.WithSyncer(syncer),
		node.WithBlockProcessor(process.BlockProcessor()),
		node.WithGenesisTime(time.Unix(nodesConfig.StartTime, 0)),
		node.WithRounder(process.Rounder()),
		node.WithShardCoordinator(shardCoordinator),
		node.WithNodesCoordinator(nodesCoordinator),
		node.WithUint64ByteSliceConverter(coreData.Uint64ByteSliceConverter()),
		node.WithSingleSigner(crypto.BlockSigner()),
		node.WithMultiSigner(crypto.MultiSigner()),
		node.WithKeyGen(keyGen),
		node.WithKeyGenForAccounts(crypto.TxSignKeyGen()),
		node.WithPubKey(pubKey),
		node.WithPrivKey(privKey),
		node.WithForkDetector(process.ForkDetector()),
		node.WithInterceptorsContainer(process.InterceptorsContainer()),
		node.WithResolversFinder(process.ResolversFinder()),
		node.WithConsensusType(config.Consensus.Type),
		node.WithTxSingleSigner(crypto.TxSingleSigner()),
		node.WithBootstrapRoundIndex(bootstrapRoundIndex),
		node.WithAppStatusHandler(coreData.StatusHandler()),
		node.WithIndexer(indexer),
		node.WithEpochStartTrigger(process.EpochStartTrigger()),
		node.WithEpochStartEventNotifier(epochStartRegistrationHandler),
		node.WithBlockBlackListHandler(process.BlackListHandler()),
		node.WithPeerDenialEvaluator(peerDenialEvaluator),
		node.WithNetworkShardingCollector(process.PeerShardMapper()),
		node.WithBootStorer(process.BootStorer()),
		node.WithRequestedItemsHandler(requestedItemsHandler),
		node.WithHeaderSigVerifier(process.HeaderSigVerifier()),
		node.WithHeaderIntegrityVerifier(process.HeaderIntegrityVerifier()),
		node.WithValidatorStatistics(process.ValidatorsStatistics()),
		node.WithValidatorsProvider(process.ValidatorsProvider()),
		node.WithChainID([]byte(coreData.ChainID())),
		node.WithMinTransactionVersion(coreData.MinTransactionVersion()),
		node.WithBlockTracker(process.BlockTracker()),
		node.WithRequestHandler(process.RequestHandler()),
		node.WithInputAntifloodHandler(network.InputAntiFloodHandler()),
		node.WithTxAccumulator(txAccumulator),
		node.WithHardforkTrigger(hardForkTrigger),
		node.WithWhiteListHandler(whiteListRequest),
		node.WithWhiteListHandlerVerified(whiteListerVerifiedTxs),
		node.WithSignatureSize(config.ValidatorPubkeyConverter.SignatureLength),
		node.WithPublicKeySize(config.ValidatorPubkeyConverter.Length),
		node.WithNodeStopChannel(chanStopNodeProcess),
		node.WithPeerHonestyHandler(peerHonestyHandler),
		node.WithWatchdogTimer(watchdogTimer),
		node.WithPeerSignatureHandler(crypto.PeerSignatureHandler()),
	)
	if err != nil {
		return nil, errors.New("error creating node: " + err.Error())
	}

	err = nd.StartHeartbeat(config.Heartbeat, version, preferencesConfig.Preferences)
	if err != nil {
		return nil, err
	}

	err = nd.ApplyOptions(node.WithDataPool(data.Datapool()))
	if err != nil {
		return nil, errors.New("error creating node: " + err.Error())
	}

	if shardCoordinator.SelfId() < shardCoordinator.NumberOfShards() {
		err = nd.CreateShardedStores()
		if err != nil {
			return nil, err
		}
	}
	if shardCoordinator.SelfId() == core.MetachainShardId {
		err = nd.ApplyOptions(node.WithPendingMiniBlocksHandler(process.PendingMiniBlocksHandler()))
		if err != nil {
			return nil, errors.New("error creating meta-node: " + err.Error())
		}
	}

	err = nodeDebugFactory.CreateInterceptedDebugHandler(
		nd,
		process.InterceptorsContainer(),
		process.ResolversFinder(),
		config.Debug.InterceptorResolver,
	)
	if err != nil {
		return nil, err
	}

	return nd, nil
}

func createPeerHonestyHandler(
	config *config.Config,
	ratingConfig config.RatingsConfig,
	pkTimeCache process.TimeCacher,
) (consensus.PeerHonestyHandler, error) {

	cache, err := storageUnit.NewCache(storageFactory.GetCacherFromConfig(config.PeerHonesty))
	if err != nil {
		return nil, err
	}

	return peerHonesty.NewP2pPeerHonesty(ratingConfig.PeerHonesty, pkTimeCache, cache)
}

func initStatsFileMonitor(
	config *config.Config,
	pubKeyString string,
	log logger.Logger,
	workingDir string,
	pathManager storage.PathManagerHandler,
	shardId string,
) error {
	statsFile, err := core.CreateFile(core.GetTrimmedPk(pubKeyString), filepath.Join(workingDir, core.DefaultStatsPath), "txt")
	if err != nil {
		return err
	}

	err = startStatisticsMonitor(statsFile, config, log, pathManager, shardId)
	if err != nil {
		return err
	}

	return nil
}

func startStatisticsMonitor(
	file *os.File,
	generalConfig *config.Config,
	log logger.Logger,
	pathManager storage.PathManagerHandler,
	shardId string,
) error {
	if !generalConfig.ResourceStats.Enabled {
		return nil
	}

	if generalConfig.ResourceStats.RefreshIntervalInSec < 1 {
		return errors.New("invalid RefreshIntervalInSec in section [ResourceStats]. Should be an integer higher than 1")
	}

	resMon, err := statistics.NewResourceMonitor(file)
	if err != nil {
		return err
	}

	go func() {
		for {
			err = resMon.SaveStatistics(generalConfig, pathManager, shardId)
			log.LogIfError(err)
			time.Sleep(time.Second * time.Duration(generalConfig.ResourceStats.RefreshIntervalInSec))
		}
	}()

	return nil
}

func createApiResolver(
	config *config.Config,
	accnts state.AccountsAdapter,
	validatorAccounts state.AccountsAdapter,
	pubkeyConv core.PubkeyConverter,
	storageService dataRetriever.StorageService,
	blockChain data.ChainHandler,
	marshalizer marshal.Marshalizer,
	hasher hashing.Hasher,
	uint64Converter typeConverters.Uint64ByteSliceConverter,
	shardCoordinator sharding.Coordinator,
	statusMetrics external.StatusMetricsHandler,
	gasSchedule map[string]map[string]uint64,
	economics *economics.EconomicsData,
	messageSigVerifier vm.MessageSignVerifier,
	nodesSetup sharding.GenesisNodesSetupHandler,
	systemSCConfig *config.SystemSmartContractsConfig,
) (facade.ApiResolver, error) {
	var vmFactory process.VirtualMachinesContainerFactory
	var err error

	argsBuiltIn := builtInFunctions.ArgsCreateBuiltInFunctionContainer{
		GasMap:          gasSchedule,
		MapDNSAddresses: make(map[string]struct{}),
		Marshalizer:     marshalizer,
	}
	builtInFuncs, err := builtInFunctions.CreateBuiltInFunctionContainer(argsBuiltIn)
	if err != nil {
		return nil, err
	}

	argsHook := hooks.ArgBlockChainHook{
		Accounts:         accnts,
		PubkeyConv:       pubkeyConv,
		StorageService:   storageService,
		BlockChain:       blockChain,
		ShardCoordinator: shardCoordinator,
		Marshalizer:      marshalizer,
		Uint64Converter:  uint64Converter,
		BuiltInFunctions: builtInFuncs,
	}

	if shardCoordinator.SelfId() == core.MetachainShardId {
		vmFactory, err = metachain.NewVMContainerFactory(
			argsHook,
			economics,
			messageSigVerifier,
			gasSchedule,
			nodesSetup,
			hasher,
			marshalizer,
			systemSCConfig,
			validatorAccounts,
		)
		if err != nil {
			return nil, err
		}
	} else {
		vmFactory, err = shard.NewVMContainerFactory(
			config.VirtualMachineConfig,
			economics.MaxGasLimitPerBlock(shardCoordinator.SelfId()),
			gasSchedule,
			argsHook)
		if err != nil {
			return nil, err
		}
	}

	vmContainer, err := vmFactory.Create()
	if err != nil {
		return nil, err
	}

	scQueryService, err := smartContract.NewSCQueryService(vmContainer, economics)
	if err != nil {
		return nil, err
	}

	argsTxTypeHandler := coordinator.ArgNewTxTypeHandler{
		PubkeyConverter:  pubkeyConv,
		ShardCoordinator: shardCoordinator,
		BuiltInFuncNames: builtInFuncs.Keys(),
		ArgumentParser:   parsers.NewCallArgsParser(),
	}
	txTypeHandler, err := coordinator.NewTxTypeHandler(argsTxTypeHandler)
	if err != nil {
		return nil, err
	}

	txCostHandler, err := transaction.NewTransactionCostEstimator(txTypeHandler, economics, scQueryService, gasSchedule)
	if err != nil {
		return nil, err
	}

	return external.NewNodeApiResolver(scQueryService, statusMetrics, txCostHandler)
}

func createWhiteListerVerifiedTxs(generalConfig *config.Config) (process.WhiteListHandler, error) {
	whiteListCacheVerified, err := storageUnit.NewCache(storageFactory.GetCacherFromConfig(generalConfig.WhiteListerVerifiedTxs))
	if err != nil {
		return nil, err
	}
	return interceptors.NewWhiteListDataVerifier(whiteListCacheVerified)
}

func updateLogger(workingDir string, ctx *cli.Context, log logger.Logger) (*os.File, error) {
	var logFile *os.File
	var err error
	withLogFile := ctx.GlobalBool(logSaveFile.Name)
	if withLogFile {
		logFile, err = prepareLogFile(workingDir)
		if err != nil {
			return nil, fmt.Errorf("%w creating a log file", err)
		}
	}

	err = logger.SetDisplayByteSlice(logger.ToHex)
	log.LogIfError(err)
	logger.ToggleCorrelation(ctx.GlobalBool(logWithCorrelation.Name))
	logger.ToggleLoggerName(ctx.GlobalBool(logWithLoggerName.Name))
	logLevelFlagValue := ctx.GlobalString(logLevel.Name)
	err = logger.SetLogLevel(logLevelFlagValue)
	if err != nil {
		return nil, err
	}
	noAnsiColor := ctx.GlobalBool(disableAnsiColor.Name)
	if noAnsiColor {
		err = logger.RemoveLogObserver(os.Stdout)
		if err != nil {
			//we need to print this manually as we do not have console log observer
			fmt.Println("error removing log observer: " + err.Error())
			return nil, err
		}

		err = logger.AddLogObserver(os.Stdout, &logger.PlainFormatter{})
		if err != nil {
			//we need to print this manually as we do not have console log observer
			fmt.Println("error setting log observer: " + err.Error())
			return nil, err
		}
	}
	log.Trace("logger updated", "level", logLevelFlagValue, "disable ANSI color", noAnsiColor)

	return logFile, nil
}

func readConfigs(log logger.Logger, ctx *cli.Context) (*configs, error) {
	log.Trace("reading configs")

	configurationFileName := ctx.GlobalString(configurationFile.Name)
	generalConfig, err := loadMainConfig(configurationFileName)
	if err != nil {
		return nil, err
	}
	log.Debug("config", "file", configurationFileName)

	configurationApiFileName := ctx.GlobalString(configurationApiFile.Name)
	apiRoutesConfig, err := loadApiConfig(configurationApiFileName)
	if err != nil {
		return nil, err
	}
	log.Debug("config", "file", configurationApiFileName)

	configurationEconomicsFileName := ctx.GlobalString(configurationEconomicsFile.Name)
	economicsConfig, err := loadEconomicsConfig(configurationEconomicsFileName)
	if err != nil {
		return nil, err
	}
	log.Debug("config", "file", configurationEconomicsFileName)

	configurationSystemSCConfigFileName := ctx.GlobalString(configurationSystemSCFile.Name)
	systemSCConfig, err := loadSystemSmartContractsConfig(configurationSystemSCConfigFileName)
	if err != nil {
		return nil, err
	}
	log.Debug("config", "file", configurationSystemSCConfigFileName)

	configurationRatingsFileName := ctx.GlobalString(configurationRatingsFile.Name)
	ratingsConfig, err := loadRatingsConfig(configurationRatingsFileName)
	if err != nil {
		return nil, err
	}
	log.Debug("config", "file", configurationRatingsFileName)

	configurationPreferencesFileName := ctx.GlobalString(configurationPreferencesFile.Name)
	preferencesConfig, err := loadPreferencesConfig(configurationPreferencesFileName)
	if err != nil {
		return nil, err
	}
	log.Debug("config", "file", configurationPreferencesFileName)

	externalConfigurationFileName := ctx.GlobalString(externalConfigFile.Name)
	externalConfig, err := loadExternalConfig(externalConfigurationFileName)
	if err != nil {
		return nil, err
	}
	log.Debug("config", "file", externalConfigurationFileName)

	p2pConfigurationFileName := ctx.GlobalString(p2pConfigurationFile.Name)
	p2pConfig, err := core.LoadP2PConfig(p2pConfigurationFileName)
	if err != nil {
		return nil, err
	}

	log.Debug("config", "file", p2pConfigurationFileName)
	if ctx.IsSet(port.Name) {
		p2pConfig.Node.Port = ctx.GlobalString(port.Name)
	}

	return &configs{
		generalConfig:                    generalConfig,
		apiRoutesConfig:                  apiRoutesConfig,
		economicsConfig:                  economicsConfig,
		systemSCConfig:                   systemSCConfig,
		ratingsConfig:                    ratingsConfig,
		preferencesConfig:                preferencesConfig,
		externalConfig:                   externalConfig,
		p2pConfig:                        p2pConfig,
		configurationFileName:            configurationFileName,
		configurationEconomicsFileName:   configurationEconomicsFileName,
		configurationRatingsFileName:     configurationRatingsFileName,
		configurationPreferencesFileName: configurationPreferencesFileName,
		p2pConfigurationFileName:         p2pConfigurationFileName,
	}, nil
}

// PrepareOpenTopics will set to the anti flood handler the topics for which
// the node can receive messages from others than validators
func PrepareOpenTopics(
	antiflood mainFactory.P2PAntifloodHandler,
	shardCoordinator sharding.Coordinator,
) {
	selfID := shardCoordinator.SelfId()
	if selfID == core.MetachainShardId {
		antiflood.SetTopicsForAll(core.HeartbeatTopic)
		return
	}

	selfShardTxTopic := processFactory.TransactionTopic + core.CommunicationIdentifierBetweenShards(selfID, selfID)
	antiflood.SetTopicsForAll(core.HeartbeatTopic, selfShardTxTopic)
}
