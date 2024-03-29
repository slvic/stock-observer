package binance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/slvic/stock-observer/internal/configs"
	"github.com/slvic/stock-observer/pkg/markets/models"
	"golang.org/x/sync/errgroup"
)

func init() {
	prometheus.MustRegister(binancePrice)
	prometheus.MustRegister(binanceTradableQuantity)
	prometheus.MustRegister(binanceCommissionRate)
}

var (
	binancePriceSummaryOpts = prometheus.SummaryOpts{
		Namespace: "binance",
		Name:      "price",
	}
	binanceTradableQuantitySummaryOpts = prometheus.SummaryOpts{
		Namespace: "binance",
		Name:      "tradableQuantity",
	}
	binanceCommissionRateSummaryOpts = prometheus.SummaryOpts{
		Namespace: "binance",
		Name:      "commissionRate",
	}
	binanceLabels = []string{"tradeType", "asset", "fiat"}
)

var (
	binancePrice = prometheus.NewSummaryVec(
		binancePriceSummaryOpts,
		binanceLabels,
	)
	binanceTradableQuantity = prometheus.NewSummaryVec(
		binanceTradableQuantitySummaryOpts,
		binanceLabels,
	)
	binanceCommissionRate = prometheus.NewSummaryVec(
		binanceCommissionRateSummaryOpts,
		binanceLabels,
	)
)

type Binance struct {
	config     configs.Binance
	httpClient http.Client
}

func New(cfg configs.Binance) *Binance {
	return &Binance{
		config:     cfg,
		httpClient: http.Client{Timeout: 15 * time.Second},
	}
}

func getOptions(asset, fiat string) []models.BinanceRequest {
	return []models.BinanceRequest{
		{
			Asset:         asset,
			Fiat:          fiat,
			MerchantCheck: true,
			Page:          1,
			PublisherType: nil,
			Rows:          20,
			TradeType:     "BUY",
		},
		{
			Asset:         asset,
			Fiat:          fiat,
			MerchantCheck: true,
			Page:          1,
			PublisherType: nil,
			Rows:          20,
			TradeType:     "SELL",
		},
	}
}

func (b *Binance) GetAllData(ctx context.Context) {
	log.Printf("binance data gathering started")

	binanceRequest, ctx := errgroup.WithContext(ctx)
	for _, fiat := range b.config.Fiats {
		for _, asset := range b.config.Assets {
			options := getOptions(asset, fiat)
			for _, option := range options {
				binanceRequest.Go(func() error {
					err := b.getData(&option)
					if err != nil {
						log.Printf("could not get binance data: %s", err.Error())
					}
					return nil
				})
			}

		}
	}
	if err := binanceRequest.Wait(); err != nil {
		log.Printf("binance api data gathered with errors: %s", err.Error())
		return
	}
	log.Printf("binance api data is successfully gathered: %v", time.Now())
}

func (b *Binance) getData(options *models.BinanceRequest) error {
	var binanceResponse models.BinanceResponse

	response, err := b.sendRequest(options)
	if err != nil {
		return fmt.Errorf("could not send request: %s", err.Error())
	}

	err = json.Unmarshal(response, &binanceResponse)
	if err != nil {
		return fmt.Errorf("could not unmarshal responce body: %s", err.Error())
	}

	for _, data := range binanceResponse.Data {
		{ //price
			price, err := strconv.ParseFloat(*data.Adv.Price, 64)
			if err != nil {
				return fmt.Errorf("could not parse the price")
			}
			binancePrice.WithLabelValues([]string{
				*data.Adv.TradeType,
				*data.Adv.Asset,
				*data.Adv.FiatUnit,
			}...).Observe(price)
		}
		{ //tradableQuantity
			tradableQuantity, err := strconv.ParseFloat(*data.Adv.TradableQuantity, 64)
			if err != nil {
				return fmt.Errorf("could not parse the price")
			}
			binanceTradableQuantity.WithLabelValues([]string{
				*data.Adv.TradeType,
				*data.Adv.Asset,
				*data.Adv.FiatUnit,
			}...).Observe(tradableQuantity)
		}
		{ //commissionRate
			commissionRate, err := strconv.ParseFloat(*data.Adv.CommissionRate, 64)
			if err != nil {
				return fmt.Errorf("could not parse the price")
			}
			binanceCommissionRate.WithLabelValues([]string{
				*data.Adv.TradeType,
				*data.Adv.Asset,
				*data.Adv.FiatUnit,
			}...).Observe(commissionRate)
		}
	}

	return nil
}

func (b Binance) sendRequest(options *models.BinanceRequest) ([]byte, error) {
	bodyBytes, err := json.Marshal(&options)
	if err != nil {
		return nil, fmt.Errorf("could not marshal options: %s", err.Error())
	}
	bodyReader := bytes.NewReader(bodyBytes)

	response, err := b.httpClient.Post(b.config.Address, "application/json", bodyReader)
	if err != nil {
		return nil, fmt.Errorf("could not send a request: %s", err.Error())
	}

	responseBodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("could not read a responce body: %s", err.Error())
	}

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unsuccessfull request, status code %d, response body: %s",
			response.StatusCode,
			string(responseBodyBytes))
	}

	return responseBodyBytes, nil
}
