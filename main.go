package main

import (
	"errors"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/codahale/metrics"
	"github.com/honeycombio/honeycomb-tcpagent/protocols/mongodb"
	"github.com/honeycombio/honeycomb-tcpagent/protocols/mysql"
	"github.com/honeycombio/honeycomb-tcpagent/publish"
	"github.com/honeycombio/honeycomb-tcpagent/sniffer"

	flag "github.com/jessevdk/go-flags"
)

type GlobalOptions struct {
	Help           bool            `short:"h" long:"help" description:"Show this help message"`
	Debug          bool            `long:"debug" description:"Print verbose debug logs"`
	MySQL          mysql.Options   `group:"MySQL parser options" namespace:"mysql"`
	MongoDB        mongodb.Options `group:"MongoDB parser options" namespace:"mongodb"`
	Sniffer        sniffer.Options `group:"Packet capture options" namespace:"capture"`
	ParserName     string          `short:"p" long:"parser" default:"mongodb" description:"Which protocol to parse (MySQL or MongoDB)"` // TODO: just support both!
	StatusInterval int             `long:"status_interval" default:"60" description:"How frequently to print summary statistics, in seconds"`
}

func main() {
	options, err := parseFlags()
	if err != nil {
		os.Exit(1)
	}
	configureLogging(options.Debug)
	go logMetrics(options.StatusInterval)
	err = run(options)
	if err != nil {
		os.Exit(1)
	}
}

func configureLogging(debug bool) {
	if debug {
		logrus.SetLevel(logrus.DebugLevel)
	}
	logrus.SetFormatter(&logrus.JSONFormatter{})
}

func run(options *GlobalOptions) error {
	var pf sniffer.ConsumerFactory
	if options.ParserName == "mysql" {
		pf = &mysql.ParserFactory{Options: options.MySQL}
	} else if options.ParserName == "mongodb" {
		pf = &mongodb.ParserFactory{
			Options:   options.MongoDB,
			Publisher: publish.NewBufferedPublisher(1024),
		}
	} else {
		log.Printf("`%s` isn't a supported parser name.\n", options.ParserName)
		log.Println("Valid parsers are `mongodb` and `mysql`.")
		os.Exit(1)
	}

	sniffer, err := sniffer.New(options.Sniffer, pf)
	if err != nil {
		log.Println("Failed to configure listener.")
		log.Printf("Error: %s\n", err)
		return err
	}
	log.Println("Listening for traffic")
	sniffer.Run()
	return nil
}

func parseFlags() (*GlobalOptions, error) {
	var options GlobalOptions
	flagParser := flag.NewParser(&options, flag.Default)
	extraArgs, err := flagParser.Parse()

	if err != nil {
		if flagErr, ok := err.(*flag.Error); ok && flagErr.Type == flag.ErrHelp {
			os.Exit(0)
		} else {
			os.Exit(1)
		}
	} else if len(extraArgs) != 0 {
		log.Printf("Unexpected extra arguments: %s", strings.Join(extraArgs, " "))
		return nil, errors.New("")
	}

	return &options, nil
}

func logMetrics(interval int) {
	ticker := time.NewTicker(time.Second * time.Duration(interval))
	for range ticker.C {
		counters, gauges := metrics.Snapshot()
		logger := logrus.WithFields(logrus.Fields{})
		for k, v := range counters {
			logger = logger.WithField(k, v)
		}
		for k, v := range gauges {
			logger = logger.WithField(k, v)
		}
		logger.Info("honeycomb-tcpagent statistics")
	}
}
