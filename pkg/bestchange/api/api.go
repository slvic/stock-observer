package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/mehanizm/iuliia-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/slvic/stock-observer/internal/configs"
	"github.com/slvic/stock-observer/pkg/bestchange/models"
	"golang.org/x/sync/errgroup"
)

func init() {
	prometheus.MustRegister(bestchageGiveRate)
	prometheus.MustRegister(bestchageGetRate)
}

var (
	bceGiveRateSummaryOpts = prometheus.SummaryOpts{
		Namespace: "bestchange",
		Name:      "exchangeGiveRate",
	}
	bceGetRateSummaryOpts = prometheus.SummaryOpts{
		Namespace: "bestchange",
		Name:      "exchangeGetRate",
	}

	bcLabels = []string{"exchanger", "source", "target"}
)

var (
	bestchageGiveRate = prometheus.NewSummaryVec(
		bceGiveRateSummaryOpts,
		bcLabels,
	)
	bestchageGetRate = prometheus.NewSummaryVec(
		bceGetRateSummaryOpts,
		bcLabels,
	)
)

type Bestchange struct {
	config     configs.Bestchange
	httpClient http.Client
}

func NewBestchangeParser(cfg configs.Bestchange) *Bestchange {
	return &Bestchange{
		config:     cfg,
		httpClient: http.Client{Timeout: 15 * time.Second},
	}
}

func (b Bestchange) GetData(ctx context.Context) {
	log.Printf("bestchange api data gathering started")

	err := b.getBcApiFile()
	if err != nil {
		log.Printf("could not get bestchange api file: %s", err.Error())
		return
	}

	err = unzipSource(bcApiZipFileName, bcApiFolder)
	if err != nil {
		log.Printf("could not unzip bestchange api file: %s", err.Error())
		return
	}

	rawGetter, _ := errgroup.WithContext(ctx)

	rawCurrencies := make(chan map[int]string, 1)
	rawExchangers := make(chan map[int]string, 1)
	rawExchangeRates := make(chan []models.RawExchangeRate, 1)

	rawGetter.Go(func() error {
		currencies, err := getRawCurrencies(currenciesFile)
		if err != nil {
			return fmt.Errorf("could not get raw currencies: %w", err)
		}
		rawCurrencies <- currencies
		return nil
	})
	rawGetter.Go(func() error {
		exchangers, err := getRawExchangers(exchangerOfficesFile)
		if err != nil {
			return fmt.Errorf("could not get raw exchangers: %w", err)
		}
		rawExchangers <- exchangers
		return nil
	})
	rawGetter.Go(func() error {
		exchangeRates, err := getRawExchangeRates(exchangeRatesFile)
		if err != nil {
			return fmt.Errorf("could not get raw exchange rates: %w", err)
		}
		rawExchangeRates <- exchangeRates
		return nil
	})

	if err = rawGetter.Wait(); err != nil {
		log.Printf("could not get raw bestchange data: %s", err.Error())
		return
	}

	exchangeRates := getExchangeRates(<-rawExchangeRates, <-rawExchangers, <-rawCurrencies)

	replacer := strings.NewReplacer(" ", "_", "-", "_", "(", "", ")", "", "/", "", ".", "")
	for _, exchangeRate := range exchangeRates {
		{ //give rate
			bestchageGiveRate.WithLabelValues([]string{
				replacer.Replace(iuliia.Wikipedia.Translate(exchangeRate.ExchangerName)),
				replacer.Replace(iuliia.Wikipedia.Translate(exchangeRate.SourceCurrency)),
				replacer.Replace(iuliia.Wikipedia.Translate(exchangeRate.TargetCurrency)),
			}...).Observe(exchangeRate.GiveRate)
		}
		{ //get rate
			bestchageGetRate.WithLabelValues([]string{
				replacer.Replace(iuliia.Wikipedia.Translate(exchangeRate.ExchangerName)),
				replacer.Replace(iuliia.Wikipedia.Translate(exchangeRate.SourceCurrency)),
				replacer.Replace(iuliia.Wikipedia.Translate(exchangeRate.TargetCurrency)),
			}...).Observe(exchangeRate.GetRate)
		}
	}
	log.Printf("bestchange api data is successfully gathered: %v", time.Now())
}
