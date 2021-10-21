package pool

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/btcsuite/btcutil"
	"github.com/lightningnetwork/lnd/cert"
	"github.com/lightningnetwork/lnd/lncfg"
	"github.com/lightningnetwork/lnd/lnrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var (
	// DefaultBaseDir is the default root data directory where pool will
	// store all its data. On UNIX like systems this will resolve to
	// ~/.pool. Below this directory the logs and network directory will be
	// created.
	DefaultBaseDir = btcutil.AppDataDir("pool", false)

	// DefaultNetwork is the default bitcoin network pool runs on.
	DefaultNetwork = "mainnet"

	// DefaultLogFilename is the default name that is given to the pool log
	// file.
	DefaultLogFilename = "poold.log"

	defaultLogLevel   = "info"
	defaultLogDirname = "logs"
	defaultLogDir     = filepath.Join(DefaultBaseDir, defaultLogDirname)

	defaultMaxLogFiles    = 3
	defaultMaxLogFileSize = 10

	defaultMinBackoff = 5 * time.Second
	defaultMaxBackoff = 1 * time.Minute

	// DefaultTLSCertFilename is the default file name for the autogenerated
	// TLS certificate.
	DefaultTLSCertFilename = "tls.cert"

	// DefaultTLSKeyFilename is the default file name for the autogenerated
	// TLS key.
	DefaultTLSKeyFilename = "tls.key"

	defaultSelfSignedOrganization = "pool autogenerated cert"

	// defaultLndMacaroon is the default macaroon file we use if the old,
	// deprecated --lnd.macaroondir config option is used.
	defaultLndMacaroon = "admin.macaroon"

	// DefaultTLSCertPath is the default full path of the autogenerated TLS
	// certificate.
	DefaultTLSCertPath = filepath.Join(
		DefaultBaseDir, DefaultNetwork, DefaultTLSCertFilename,
	)

	// DefaultTLSKeyPath is the default full path of the autogenerated TLS
	// key.
	DefaultTLSKeyPath = filepath.Join(
		DefaultBaseDir, DefaultNetwork, DefaultTLSKeyFilename,
	)

	// DefaultMacaroonFilename is the default file name for the
	// autogenerated pool macaroon.
	DefaultMacaroonFilename = "pool.macaroon"

	// DefaultMacaroonPath is the default full path of the base pool
	// macaroon.
	DefaultMacaroonPath = filepath.Join(
		DefaultBaseDir, DefaultNetwork, DefaultMacaroonFilename,
	)

	// DefaultLndDir is the default location where we look for lnd's tls and
	// macaroon files.
	DefaultLndDir = btcutil.AppDataDir("lnd", false)

	// DefaultLndMacaroonPath is the default location where we look for a
	// macaroon to use when connecting to lnd.
	DefaultLndMacaroonPath = filepath.Join(
		DefaultLndDir, "data", "chain", "bitcoin", DefaultNetwork,
		defaultLndMacaroon,
	)

	// DefaultAutogenValidity is the default validity of a self-signed
	// certificate. The value corresponds to 14 months
	// (14 months * 30 days * 24 hours).
	DefaultAutogenValidity = 14 * 30 * 24 * time.Hour
)

type LndConfig struct {
	Host string `long:"host" description:"lnd instance rpc address"`

	// MacaroonDir is the directory that contains all the macaroon files
	// required for the remote connection.
	MacaroonDir string `long:"macaroondir" description:"DEPRECATED: Use macaroonpath."`

	// MacaroonPath is the path to the single macaroon that should be used
	// instead of needing to specify the macaroon directory that contains
	// all of lnd's macaroons. The specified macaroon MUST have all
	// permissions that all the subservers use, otherwise permission errors
	// will occur.
	MacaroonPath string `long:"macaroonpath" description:"The full path to the single macaroon to use, either the admin.macaroon or a custom baked one. Cannot be specified at the same time as macaroondir. A custom macaroon must contain ALL permissions required for all subservers to work, otherwise permission errors will occur."`

	TLSPath string `long:"tlspath" description:"Path to lnd tls certificate"`
}

type Config struct {
	ShowVersion    bool   `long:"version" description:"Display version information and exit"`
	Insecure       bool   `long:"insecure" description:"disable tls"`
	Network        string `long:"network" description:"network to run on" choice:"regtest" choice:"testnet" choice:"mainnet" choice:"simnet"`
	AuctionServer  string `long:"auctionserver" description:"auction server address host:port"`
	Proxy          string `long:"proxy" description:"The host:port of a SOCKS proxy through which all connections to the pool server will be established over"`
	TLSPathAuctSrv string `long:"tlspathauctserver" description:"Path to auction server tls certificate"`
	RPCListen      string `long:"rpclisten" description:"Address to listen on for gRPC clients"`
	RESTListen     string `long:"restlisten" description:"Address to listen on for REST clients"`
	BaseDir        string `long:"basedir" description:"The base directory where pool stores all its data. If set, this option overwrites --logdir, --macaroonpath, --tlscertpath and --tlskeypath."`

	LogDir         string `long:"logdir" description:"Directory to log output."`
	MaxLogFiles    int    `long:"maxlogfiles" description:"Maximum logfiles to keep (0 for no rotation)"`
	MaxLogFileSize int    `long:"maxlogfilesize" description:"Maximum logfile size in MB"`

	MinBackoff time.Duration `long:"minbackoff" description:"Shortest backoff when reconnecting to the server. Valid time units are {s, m, h}."`
	MaxBackoff time.Duration `long:"maxbackoff" description:"Longest backoff when reconnecting to the server. Valid time units are {s, m, h}."`
	DebugLevel string        `long:"debuglevel" description:"Logging level for all subsystems {trace, debug, info, warn, error, critical} -- You may also specify <subsystem>=<level>,<subsystem2>=<level>,... to set the log level for individual subsystems -- Use show to list available subsystems"`

	TLSCertPath        string   `long:"tlscertpath" description:"Path to write the TLS certificate for pool's RPC and REST services."`
	TLSKeyPath         string   `long:"tlskeypath" description:"Path to write the TLS private key for pool's RPC and REST services."`
	TLSExtraIPs        []string `long:"tlsextraip" description:"Adds an extra IP to the generated certificate."`
	TLSExtraDomains    []string `long:"tlsextradomain" description:"Adds an extra domain to the generated certificate."`
	TLSAutoRefresh     bool     `long:"tlsautorefresh" description:"Re-generate TLS certificate and key if the IPs or domains are changed."`
	TLSDisableAutofill bool     `long:"tlsdisableautofill" description:"Do not include the interface IPs or the system hostname in TLS certificate, use first --tlsextradomain as Common Name instead, if set."`

	MacaroonPath string `long:"macaroonpath" description:"Path to write the macaroon for pool's RPC and REST services if it doesn't exist."`

	NewNodesOnly bool `long:"newnodesonly" description:"Only accept channels from nodes that the connected lnd node doesn't already have open or pending channels with."`

	LsatMaxRoutingFee btcutil.Amount `long:"lsatmaxroutingfee" description:"The maximum amount in satoshis we are willing to pay in routing fees when paying for the one-time LSAT auth token that is required to use the Pool service."`

	Profile  string `long:"profile" description:"Enable HTTP profiling on given port -- NOTE port must be between 1024 and 65535"`
	FakeAuth bool   `long:"fakeauth" description:"Disable LSAT authentication and instead use a fake LSAT ID to identify. For testing only, cannot be set on mainnet."`

	TxLabelPrefix string `long:"txlabelprefix" description:"If set, then every transaction poold makes will be created with a label that has this string as a prefix."`

	Lnd *LndConfig `group:"lnd" namespace:"lnd"`

	// RPCListener is a network listener that can be set if poold should be
	// used as a library and listen on the given listener instead of what is
	// configured in the --rpclisten parameter. Setting this will also
	// disable REST.
	RPCListener net.Listener

	// AuctioneerDialOpts is a list of dial options that should be used when
	// dialing the auctioneer server.
	AuctioneerDialOpts []grpc.DialOption
}

const (
	MainnetServer = "pool.lightning.finance:12010"
	TestnetServer = "test.pool.lightning.finance:12010"

	// defaultRPCTimeout is the default number of seconds an unary RPC call
	// is allowed to take to complete.
	defaultRPCTimeout  = 30 * time.Second
	defaultLsatMaxCost = btcutil.Amount(1000)
	defaultLsatMaxFee  = btcutil.Amount(50)
)

// DefaultConfig returns the default value for the Config struct.
func DefaultConfig() Config {
	return Config{
		Network:           DefaultNetwork,
		RPCListen:         "localhost:12010",
		RESTListen:        "localhost:8281",
		Insecure:          false,
		BaseDir:           DefaultBaseDir,
		LogDir:            defaultLogDir,
		MaxLogFiles:       defaultMaxLogFiles,
		MaxLogFileSize:    defaultMaxLogFileSize,
		MinBackoff:        defaultMinBackoff,
		MaxBackoff:        defaultMaxBackoff,
		DebugLevel:        defaultLogLevel,
		TLSCertPath:       DefaultTLSCertPath,
		TLSKeyPath:        DefaultTLSKeyPath,
		MacaroonPath:      DefaultMacaroonPath,
		LsatMaxRoutingFee: defaultLsatMaxFee,
		Lnd: &LndConfig{
			Host:         "localhost:10009",
			MacaroonPath: DefaultLndMacaroonPath,
		},
	}
}

// Validate cleans up paths in the config provided and validates it.
func Validate(cfg *Config) error {
	// Cleanup any paths before we use them.
	cfg.BaseDir = lncfg.CleanAndExpandPath(cfg.BaseDir)
	cfg.LogDir = lncfg.CleanAndExpandPath(cfg.LogDir)
	cfg.TLSCertPath = lncfg.CleanAndExpandPath(cfg.TLSCertPath)
	cfg.TLSKeyPath = lncfg.CleanAndExpandPath(cfg.TLSKeyPath)
	cfg.MacaroonPath = lncfg.CleanAndExpandPath(cfg.MacaroonPath)

	// Since our pool directory overrides our log and TLS dir values, make
	// sure that they are not set when base dir is set. We hard here rather
	// than overwriting and potentially confusing the user.
	baseDirSet := cfg.BaseDir != DefaultBaseDir

	if baseDirSet {
		logDirSet := cfg.LogDir != defaultLogDir
		tlsCertPathSet := cfg.TLSCertPath != DefaultTLSCertPath
		tlsKeyPathSet := cfg.TLSKeyPath != DefaultTLSKeyPath
		macaroonPathSet := cfg.MacaroonPath != DefaultMacaroonPath

		if logDirSet {
			return fmt.Errorf("basedir overwrites logdir, please " +
				"only set one value")
		}

		if tlsCertPathSet {
			return fmt.Errorf("basedir overwrites tlscertpath, " +
				"please only set one value")
		}

		if tlsKeyPathSet {
			return fmt.Errorf("basedir overwrites tlskeypath, " +
				"please only set one value")
		}

		if macaroonPathSet {
			return fmt.Errorf("basedir overwrites macaroonpath, " +
				"please only set one value")
		}

		// Once we are satisfied that no other config value was set, we
		// replace them with our pool dir.
		cfg.LogDir = filepath.Join(cfg.BaseDir, defaultLogDirname)
	}

	// Append the network type to the log and base directory so it is
	// "namespaced" per network in the same fashion as the data directory.
	cfg.LogDir = filepath.Join(cfg.LogDir, cfg.Network)
	cfg.BaseDir = filepath.Join(cfg.BaseDir, cfg.Network)

	// We want the TLS and macaroon files to also be in the "namespaced" sub
	// directory. Replace the default values with actual values in case the
	// user specified basedir.
	if cfg.TLSCertPath == DefaultTLSCertPath {
		cfg.TLSCertPath = filepath.Join(
			cfg.BaseDir, DefaultTLSCertFilename,
		)
	}
	if cfg.TLSKeyPath == DefaultTLSKeyPath {
		cfg.TLSKeyPath = filepath.Join(
			cfg.BaseDir, DefaultTLSKeyFilename,
		)
	}
	if cfg.MacaroonPath == DefaultMacaroonPath {
		cfg.MacaroonPath = filepath.Join(
			cfg.BaseDir, DefaultMacaroonFilename,
		)
	}

	// If either of these directories do not exist, create them.
	if err := os.MkdirAll(cfg.BaseDir, os.ModePerm); err != nil {
		return err
	}

	if err := os.MkdirAll(cfg.LogDir, os.ModePerm); err != nil {
		return err
	}

	// Make sure only one of the macaroon options is used.
	switch {
	case cfg.Lnd.MacaroonPath != DefaultLndMacaroonPath &&
		cfg.Lnd.MacaroonDir != "":

		return fmt.Errorf("use --lnd.macaroonpath only")

	case cfg.Lnd.MacaroonDir != "":
		// With the new version of lndclient we can only specify a
		// single macaroon instead of all of them. If the old
		// macaroondir is used, we use the admin macaroon located in
		// that directory.
		cfg.Lnd.MacaroonPath = path.Join(
			lncfg.CleanAndExpandPath(cfg.Lnd.MacaroonDir),
			defaultLndMacaroon,
		)

	case cfg.Lnd.MacaroonPath != "":
		cfg.Lnd.MacaroonPath = lncfg.CleanAndExpandPath(
			cfg.Lnd.MacaroonPath,
		)

	default:
		return fmt.Errorf("must specify --lnd.macaroonpath")
	}

	// Adjust the default lnd macaroon path if only the network is
	// specified.
	if cfg.Network != DefaultNetwork &&
		cfg.Lnd.MacaroonPath == DefaultLndMacaroonPath {

		cfg.Lnd.MacaroonPath = path.Join(
			DefaultLndDir, "data", "chain", "bitcoin", cfg.Network,
			defaultLndMacaroon,
		)
	}

	return nil
}

// getTLSConfig generates a new self signed certificate or refreshes an existing
// one if necessary, then returns the full TLS configuration for initializing
// a secure server interface.
func getTLSConfig(cfg *Config) (*tls.Config, *credentials.TransportCredentials,
	error) {

	// Let's load our certificate first or create then load if it doesn't
	// yet exist.
	certData, parsedCert, err := loadCertWithCreate(cfg)
	if err != nil {
		return nil, nil, err
	}

	// If the certificate expired or it was outdated, delete it and the TLS
	// key and generate a new pair.
	if time.Now().After(parsedCert.NotAfter) {
		log.Info("TLS certificate is expired or outdated, " +
			"removing old file then generating a new one")

		err := os.Remove(cfg.TLSCertPath)
		if err != nil {
			return nil, nil, err
		}

		err = os.Remove(cfg.TLSKeyPath)
		if err != nil {
			return nil, nil, err
		}

		certData, _, err = loadCertWithCreate(cfg)
		if err != nil {
			return nil, nil, err
		}
	}

	tlsCfg := cert.TLSConfFromCert(certData)
	tlsCfg.NextProtos = []string{"h2"}
	restCreds, err := credentials.NewClientTLSFromFile(
		cfg.TLSCertPath, "",
	)
	if err != nil {
		return nil, nil, err
	}

	return tlsCfg, &restCreds, nil
}

// loadCertWithCreate tries to load the TLS certificate from disk. If the
// specified cert and key files don't exist, the certificate/key pair is created
// first.
func loadCertWithCreate(cfg *Config) (tls.Certificate, *x509.Certificate,
	error) {

	// Ensure we create TLS key and certificate if they don't exist.
	if !lnrpc.FileExists(cfg.TLSCertPath) &&
		!lnrpc.FileExists(cfg.TLSKeyPath) {

		log.Infof("Generating TLS certificates...")
		err := cert.GenCertPair(
			defaultSelfSignedOrganization, cfg.TLSCertPath,
			cfg.TLSKeyPath, cfg.TLSExtraIPs,
			cfg.TLSExtraDomains, cfg.TLSDisableAutofill,
			DefaultAutogenValidity,
		)
		if err != nil {
			return tls.Certificate{}, nil, err
		}
		log.Infof("Done generating TLS certificates")
	}

	return cert.LoadCert(cfg.TLSCertPath, cfg.TLSKeyPath)
}
