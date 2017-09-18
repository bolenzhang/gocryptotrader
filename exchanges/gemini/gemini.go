package gemini

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/thrasher-/gocryptotrader/common"
	"github.com/thrasher-/gocryptotrader/config"
	"github.com/thrasher-/gocryptotrader/exchanges"
	"github.com/thrasher-/gocryptotrader/exchanges/ticker"
)

const (
	geminiAPIURL        = "https://api.gemini.com"
	geminiSandboxAPIURL = "https://api.sandbox.gemini.com"
	geminiAPIVersion    = "1"

	geminiSymbols            = "symbols"
	geminiTicker             = "pubticker"
	geminiAuction            = "auction"
	geminiAuctionHistory     = "history"
	geminiOrderbook          = "book"
	geminiTrades             = "trades"
	geminiOrders             = "orders"
	geminiOrderNew           = "order/new"
	geminiOrderCancel        = "order/cancel"
	geminiOrderCancelSession = "order/cancel/session"
	geminiOrderCancelAll     = "order/cancel/all"
	geminiOrderStatus        = "order/status"
	geminiMyTrades           = "mytrades"
	geminiBalances           = "balances"
	geminiTradeVolume        = "tradevolume"
	geminiDeposit            = "deposit"
	geminiNewAddress         = "newAddress"
	geminiWithdraw           = "withdraw/"
	geminiHeartbeat          = "heartbeat"

	// rate limits per minute
	geminiPublicRate  = 120
	geminiPrivateRate = 600

	// rates limits per second
	geminiPublicRateSec  = 1
	geminiPrivateRateSec = 5

	// Too many requests returns this
	geminiRateError = "429"

	// Assigned API key roles on creation
	geminiRoleTrader      = "trader"
	geminiRoleFundManager = "fundmanager"
)

// SessionID map guides
var (
	sessionAPIKey    map[int]string    // map[sessionID]APIKEY
	sessionAPISecret map[int]string    // map[sessionID]APIKEY
	sessionRole      map[string]string // map[sessionID]Roles
	sessionHeartbeat map[int]bool      // map[sessionID]RequiresHeartBeat
	IsSession        bool
)

// Gemini is the overarching type across the Gemini package, create multiple
// instances with differing APIkeys for segregation of roles for authenticated
// requests & sessions by appending the session function, if sandbox test is
// needed append the sandbox function as well.
type Gemini struct {
	exchange.Base
	M sync.Mutex
}

// AddSession adds a new session to the gemini base
func (g *Gemini) AddSession(sessionID int, apiKey, apiSecret, role string, needsHeartbeat bool) error {
	g.M.Lock()
	defer g.M.Unlock()
	if sessionAPIKey == nil {
		IsSession = true
		sessionAPIKey = make(map[int]string)
		sessionAPISecret = make(map[int]string)
		sessionRole = make(map[string]string)
		sessionHeartbeat = make(map[int]bool)
	}
	_, ok := sessionAPIKey[sessionID]
	if ok {
		return errors.New("sessionID already being used")
	}

	sessionAPIKey[sessionID] = apiKey
	sessionAPISecret[sessionID] = apiSecret
	sessionRole[apiKey] = role
	sessionHeartbeat[sessionID] = needsHeartbeat

	return nil
}

//return session function?

// SetDefaults sets package defaults for gemini exchange
func (g *Gemini) SetDefaults() {
	g.Name = "Gemini"
	g.Enabled = false
	g.Verbose = false
	g.Websocket = false
	g.RESTPollingDelay = 10
	g.RequestCurrencyPairFormat.Delimiter = ""
	g.RequestCurrencyPairFormat.Uppercase = true
	g.ConfigCurrencyPairFormat.Delimiter = ""
	g.ConfigCurrencyPairFormat.Uppercase = true
	g.AssetTypes = []string{ticker.Spot}
}

// Setup sets exchange configuration paramaters
func (g *Gemini) Setup(exch config.ExchangeConfig) {
	if !exch.Enabled {
		g.SetEnabled(false)
	} else {
		g.Enabled = true
		g.AuthenticatedAPISupport = exch.AuthenticatedAPISupport
		g.SetAPIKeys(exch.APIKey, exch.APISecret, "", false)
		g.RESTPollingDelay = exch.RESTPollingDelay
		g.Verbose = exch.Verbose
		g.Websocket = exch.Websocket
		g.BaseCurrencies = common.SplitStrings(exch.BaseCurrencies, ",")
		g.AvailablePairs = common.SplitStrings(exch.AvailablePairs, ",")
		g.EnabledPairs = common.SplitStrings(exch.EnabledPairs, ",")
		err := g.SetCurrencyPairFormat()
		if err != nil {
			log.Fatal(err)
		}
		err = g.SetAssetTypes()
		if err != nil {
			log.Fatal(err)
		}
	}
}

// Session is a session manager for differing APIKeys and roles, use this for all function
// calls in this package
func (g *Gemini) Session(sessionID int) *Gemini {
	g.M.Lock()
	defer g.M.Unlock()
	g.APIUrl = geminiAPIURL
	_, ok := sessionAPIKey[sessionID]
	if !ok {
		return nil
	}
	g.APIKey = sessionAPIKey[sessionID]
	g.APISecret = sessionAPISecret[sessionID]
	return g
}

// Sandbox diverts the apiURL to the sandbox API for testing purposes
func (g *Gemini) Sandbox() *Gemini {
	g.APIUrl = geminiSandboxAPIURL
	return g
}

// GetSymbols returns all available symbols for trading
func (g *Gemini) GetSymbols() ([]string, error) {
	symbols := []string{}
	path := fmt.Sprintf("%s/v%s/%s", geminiAPIURL, geminiAPIVersion, geminiSymbols)

	return symbols, common.SendHTTPGetRequest(path, true, g.Verbose, &symbols)
}

// GetTicker returns information about recent trading activity for the symbol
func (g *Gemini) GetTicker(currencyPair string) (Ticker, error) {

	type TickerResponse struct {
		Ask    float64 `json:"ask,string"`
		Bid    float64 `json:"bid,string"`
		Last   float64 `json:"last,string"`
		Volume map[string]interface{}
	}

	ticker := Ticker{}
	resp := TickerResponse{}
	path := fmt.Sprintf("%s/v%s/%s/%s", geminiAPIURL, geminiAPIVersion, geminiTicker, currencyPair)

	err := common.SendHTTPGetRequest(path, true, g.Verbose, &resp)
	if err != nil {
		return ticker, err
	}

	ticker.Ask = resp.Ask
	ticker.Bid = resp.Bid
	ticker.Last = resp.Last

	ticker.Volume.Currency, _ = strconv.ParseFloat(resp.Volume[currencyPair[0:3]].(string), 64)
	ticker.Volume.USD, _ = strconv.ParseFloat(resp.Volume["USD"].(string), 64)

	time, _ := resp.Volume["timestamp"].(float64)
	ticker.Volume.Timestamp = int64(time)

	return ticker, nil
}

// GetOrderbook returns the current order book, as two arrays, one of bids, and
// one of asks
//
// params - limit_bids or limit_asks [OPTIONAL] default 50, 0 returns all Values
// Type is an integer ie "params.Set("limit_asks", 30)"
func (g *Gemini) GetOrderbook(currencyPair string, params url.Values) (Orderbook, error) {
	path := common.EncodeURLValues(fmt.Sprintf("%s/v%s/%s/%s", geminiAPIURL, geminiAPIVersion, geminiOrderbook, currencyPair), params)
	orderbook := Orderbook{}

	return orderbook, common.SendHTTPGetRequest(path, true, g.Verbose, &orderbook)
}

// GetTrades eturn the trades that have executed since the specified timestamp.
// Timestamps are either seconds or milliseconds since the epoch (1970-01-01).
//
// currencyPair - example "btcusd"
// params --
// since, timestamp [optional]
// limit_trades	integer	Optional. The maximum number of trades to return.
// include_breaks	boolean	Optional. Whether to display broken trades. False by
// default. Can be '1' or 'true' to activate
func (g *Gemini) GetTrades(currencyPair string, params url.Values) ([]Trade, error) {
	path := common.EncodeURLValues(fmt.Sprintf("%s/v%s/%s/%s", geminiAPIURL, geminiAPIVersion, geminiTrades, currencyPair), params)
	trades := []Trade{}

	return trades, common.SendHTTPGetRequest(path, true, g.Verbose, &trades)
}

// GetAuction returns auction infomation
func (g *Gemini) GetAuction(currencyPair string) (Auction, error) {
	path := fmt.Sprintf("%s/v%s/%s/%s", geminiAPIURL, geminiAPIVersion, geminiAuction, currencyPair)
	auction := Auction{}

	return auction, common.SendHTTPGetRequest(path, true, g.Verbose, &auction)
}

// GetAuctionHistory returns the auction events, optionally including
// publications of indicative prices, since the specific timestamp.
//
// currencyPair - example "btcusd"
// params -- [optional]
//          since - [timestamp] Only returns auction events after the specified
// timestamp.
//          limit_auction_results - [integer] The maximum number of auction
// events to return.
//          include_indicative - [bool] Whether to include publication of
// indicative prices and quantities.
func (g *Gemini) GetAuctionHistory(currencyPair string, params url.Values) ([]AuctionHistory, error) {
	path := common.EncodeURLValues(fmt.Sprintf("%s/v%s/%s/%s/%s", geminiAPIURL, geminiAPIVersion, geminiAuction, currencyPair, geminiAuctionHistory), params)
	auctionHist := []AuctionHistory{}

	return auctionHist, common.SendHTTPGetRequest(path, true, g.Verbose, &auctionHist)
}

func (g *Gemini) isCorrectSession(role string) error {
	if !IsSession {
		return errors.New("session not set")
	}
	if sessionRole[g.APIKey] != role {
		return errors.New("incorrect role for APIKEY cannot use this function")
	}
	return nil
}

// NewOrder Only limit orders are supported through the API at present.
// returns order ID if successful
func (g *Gemini) NewOrder(symbol string, amount, price float64, side, orderType string) (int64, error) {
	if err := g.isCorrectSession(geminiRoleTrader); err != nil {
		return 0, err
	}

	request := make(map[string]interface{})
	request["symbol"] = symbol
	request["amount"] = strconv.FormatFloat(amount, 'f', -1, 64)
	request["price"] = strconv.FormatFloat(price, 'f', -1, 64)
	request["side"] = side
	request["type"] = orderType

	response := Order{}
	err := g.SendAuthenticatedHTTPRequest("POST", geminiOrderNew, request, &response)
	if err != nil {
		return 0, err
	}
	return response.OrderID, nil
}

// CancelOrder will cancel an order. If the order is already canceled, the
// message will succeed but have no effect.
func (g *Gemini) CancelOrder(OrderID int64) (Order, error) {
	request := make(map[string]interface{})
	request["order_id"] = OrderID

	response := Order{}
	err := g.SendAuthenticatedHTTPRequest("POST", geminiOrderCancel, request, &response)
	if err != nil {
		return Order{}, err
	}
	return response, nil
}

// CancelOrders will cancel all outstanding orders created by all sessions owned
// by this account, including interactive orders placed through the UI. If
// sessions = true will only cancel the order that is called on this session
// asssociated with the APIKEY
func (g *Gemini) CancelOrders(CancelBySession bool) (OrderResult, error) {
	response := OrderResult{}
	path := geminiOrderCancelAll
	if CancelBySession {
		path = geminiOrderCancelSession
	}

	return response, g.SendAuthenticatedHTTPRequest("POST", path, nil, &response)
}

// GetOrderStatus returns the status for an order
func (g *Gemini) GetOrderStatus(orderID int64) (Order, error) {
	request := make(map[string]interface{})
	request["order_id"] = orderID

	response := Order{}

	return response,
		g.SendAuthenticatedHTTPRequest("POST", geminiOrderStatus, request, &response)
}

// GetOrders returns active orders in the market
func (g *Gemini) GetOrders() ([]Order, error) {
	response := []Order{}

	return response,
		g.SendAuthenticatedHTTPRequest("POST", geminiOrders, nil, &response)
}

// GetTradeHistory returns an array of trades that have been on the exchange
//
// currencyPair - example "btcusd"
// timestamp - [optional] Only return trades on or after this timestamp.
func (g *Gemini) GetTradeHistory(currencyPair string, timestamp int64) ([]TradeHistory, error) {
	response := []TradeHistory{}
	request := make(map[string]interface{})
	request["symbol"] = currencyPair

	if timestamp != 0 {
		request["timestamp"] = timestamp
	}

	return response,
		g.SendAuthenticatedHTTPRequest("POST", geminiMyTrades, request, &response)
}

// GetTradeVolume returns a multi-arrayed volume response
func (g *Gemini) GetTradeVolume() ([][]TradeVolume, error) {
	response := [][]TradeVolume{}

	return response,
		g.SendAuthenticatedHTTPRequest("POST", geminiTradeVolume, nil, &response)
}

// GetBalances returns available balances in the supported currencies
func (g *Gemini) GetBalances() ([]Balance, error) {
	response := []Balance{}

	return response,
		g.SendAuthenticatedHTTPRequest("POST", geminiBalances, nil, &response)
}

// GetDepositAddress returns a deposit address
func (g *Gemini) GetDepositAddress(depositAddlabel, currency string) (DepositAddress, error) {
	response := DepositAddress{}

	return response,
		g.SendAuthenticatedHTTPRequest("POST", geminiDeposit+"/"+currency+"/"+geminiNewAddress, nil, &response)
}

// WithdrawCrypto withdraws crypto currency to a whitelisted address
func (g *Gemini) WithdrawCrypto(address, currency string, amount float64) (WithdrawelAddress, error) {
	response := WithdrawelAddress{}
	request := make(map[string]interface{})
	request["address"] = address
	request["amount"] = strconv.FormatFloat(amount, 'f', -1, 64)

	return response,
		g.SendAuthenticatedHTTPRequest("POST", geminiWithdraw+currency, nil, &response)
}

// PostHeartbeat sends a maintenance heartbeat to the exchange for all heartbeat
// maintaned sessions
func (g *Gemini) PostHeartbeat() (string, error) {
	type Response struct {
		Result string `json:"result"`
	}
	response := Response{}

	return response.Result,
		g.SendAuthenticatedHTTPRequest("POST", geminiHeartbeat, nil, &response)
}

// SendAuthenticatedHTTPRequest sends an authenticated HTTP request to the
// exchange and returns an error
func (g *Gemini) SendAuthenticatedHTTPRequest(method, path string, params map[string]interface{}, result interface{}) (err error) {
	if !g.AuthenticatedAPISupport {
		return fmt.Errorf(exchange.WarningAuthenticatedRequestWithoutCredentialsSet, g.Name)
	}

	if g.Nonce.Get() == 0 {
		g.Nonce.Set(time.Now().UnixNano())
	} else {
		g.Nonce.Inc()
	}

	headers := make(map[string]string)
	request := make(map[string]interface{})
	request["request"] = fmt.Sprintf("/v%s/%s", geminiAPIVersion, path)
	request["nonce"] = g.Nonce.Get()

	if params != nil {
		for key, value := range params {
			request[key] = value
		}
	}

	PayloadJSON, err := common.JSONEncode(request)
	if err != nil {
		return errors.New("SendAuthenticatedHTTPRequest: Unable to JSON request")
	}

	if g.Verbose {
		log.Printf("Request JSON: %s\n", PayloadJSON)
	}

	PayloadBase64 := common.Base64Encode(PayloadJSON)
	hmac := common.GetHMAC(common.HashSHA512_384, []byte(PayloadBase64), []byte(g.APISecret))

	headers["X-GEMINI-APIKEY"] = g.APIKey
	headers["X-GEMINI-PAYLOAD"] = PayloadBase64
	headers["X-GEMINI-SIGNATURE"] = common.HexEncodeToString(hmac)

	resp, err := common.SendHTTPRequest(method, g.APIUrl+"/v1/"+path, headers, strings.NewReader(""))
	if err != nil {
		return err
	}

	if g.Verbose {
		log.Printf("Received raw: \n%s\n", resp)
	}

	captureErr := ErrorCapture{}
	if err = common.JSONDecode([]byte(resp), &captureErr); err == nil {
		if len(captureErr.Message) != 0 || len(captureErr.Result) != 0 || len(captureErr.Reason) != 0 {
			if captureErr.Result != "ok" {
				return errors.New(captureErr.Message)
			}
		}
	}

	err = common.JSONDecode([]byte(resp), &result)
	if err != nil {
		return err
	}

	return nil
}
