package crosschain

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"strings"

	"github.com/pattonkan/sui-go/sui"
	suiclient "github.com/pattonkan/sui-go/suiclient"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// RedeemListener consumes bridge_redeem events on Sui and forwards them to the worker.
type RedeemListener interface {
	Start(ctx context.Context, handle func(context.Context, RedeemSubmission)) error
}

// SuiBridgeRedeemListener subscribes to BridgeRedeemEvent events for f/x tokens.
type SuiBridgeRedeemListener struct {
	client     *suiclient.ClientImpl
	fEventType *sui.StructTag
	xEventType *sui.StructTag
	logger     *zap.SugaredLogger
}

// NewSuiBridgeRedeemListenerFromEnv enables the listener when LFS_ENABLE_BRIDGE_REDEEM=1
// and the required Sui env vars are present.
func NewSuiBridgeRedeemListenerFromEnv(logger *zap.SugaredLogger) (*SuiBridgeRedeemListener, error) {
	if !isTruthy(strings.TrimSpace(os.Getenv("LFS_ENABLE_BRIDGE_REDEEM"))) {
		return nil, nil
	}

	rpc := strings.TrimSpace(os.Getenv("LFS_SUI_RPC_URL"))
	fToken := strings.TrimSpace(os.Getenv("LFS_SUI_FTOKEN_TYPE"))
	xToken := strings.TrimSpace(os.Getenv("LFS_SUI_XTOKEN_TYPE"))
	if rpc == "" || fToken == "" || xToken == "" {
		return nil, fmt.Errorf("redeem listener enabled but missing LFS_SUI_RPC_URL, LFS_SUI_FTOKEN_TYPE, LFS_SUI_XTOKEN_TYPE")
	}

	fPkg := parsePkg(fToken)
	xPkg := parsePkg(xToken)
	if fPkg == "" || xPkg == "" {
		return nil, fmt.Errorf("unable to parse package ids for redeem listener (%s / %s)", fToken, xToken)
	}

	fEvent, err := sui.StructTagFromString(fmt.Sprintf("%s::ftoken::BridgeRedeemEvent", fPkg))
	if err != nil {
		return nil, fmt.Errorf("parse fToken redeem event type: %w", err)
	}
	xEvent, err := sui.StructTagFromString(fmt.Sprintf("%s::xtoken::BridgeRedeemEvent", xPkg))
	if err != nil {
		return nil, fmt.Errorf("parse xToken redeem event type: %w", err)
	}

	client := suiclient.NewClient(rpc)
	wsURL := strings.TrimSpace(os.Getenv("LFS_SUI_WS_URL"))
	if wsURL == "" {
		wsURL = inferSuiWebsocketURL(rpc, os.Getenv("LFS_NETWORK"))
	}
	if wsURL == "" {
		return nil, fmt.Errorf("redeem listener enabled but missing websocket URL; set LFS_SUI_WS_URL or use a supported LFS_SUI_RPC_URL")
	}
	if err := initSuiWebsocket(client, wsURL); err != nil {
		return nil, err
	}
	logger.Infow("Bridge redeem listener enabled",
		"suiRpc", rpc,
		"suiWs", wsURL,
		"fEventType", fEvent.String(),
		"xEventType", xEvent.String(),
	)

	return &SuiBridgeRedeemListener{
		client:     client,
		fEventType: fEvent,
		xEventType: xEvent,
		logger:     logger,
	}, nil
}

// Start subscribes to both f/x BridgeRedeemEvent streams.
func (l *SuiBridgeRedeemListener) Start(ctx context.Context, handle func(context.Context, RedeemSubmission)) error {
	if l == nil || handle == nil {
		return nil
	}
	if l.client == nil {
		return fmt.Errorf("redeem listener missing sui client")
	}

	filter := l.eventFilter()
	resultCh := make(chan suiclient.Event, 32)
	if err := l.client.SubscribeEvent(ctx, filter, resultCh); err != nil {
		return fmt.Errorf("subscribe event: %w", err)
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case evt := <-resultCh:
				l.processEvent(ctx, evt, handle)
			}
		}
	}()
	return nil
}

func (l *SuiBridgeRedeemListener) eventFilter() *suiclient.EventFilter {
	all := []suiclient.EventFilter{}
	if l.fEventType != nil {
		all = append(all, suiclient.EventFilter{MoveEventType: l.fEventType})
	}
	if l.xEventType != nil {
		all = append(all, suiclient.EventFilter{MoveEventType: l.xEventType})
	}
	if len(all) == 1 {
		return &all[0]
	}
	return &suiclient.EventFilter{Any: &all}
}

func (l *SuiBridgeRedeemListener) processEvent(ctx context.Context, evt suiclient.Event, handle func(context.Context, RedeemSubmission)) {
	token := l.tokenFromEvent(evt)
	if token == "" {
		l.logger.Debugw("Ignoring event from unknown module", "eventType", evt.Type)
		return
	}

	// ParsedJson should hold {redeemer, eth_recipient, amount}
	var payload map[string]any
	rawJSON, err := json.Marshal(evt.ParsedJson)
	if err != nil {
		l.logger.Warnw("Failed to marshal bridge redeem event", "error", err)
		return
	}
	if err := json.Unmarshal(rawJSON, &payload); err != nil {
		l.logger.Warnw("Failed to decode bridge redeem event", "error", err)
		return
	}

	amountDec, err := parseAmountDecimal(payload["amount"])
	if err != nil {
		l.logger.Warnw("Failed to parse bridge redeem amount", "error", err)
		return
	}

	ethRecipient := parseEthRecipient(payload["eth_recipient"])
	if ethRecipient == "" {
		l.logger.Warnw("Missing eth_recipient in bridge redeem event")
		return
	}

	suiOwner := ""
	if evt.Sender != nil {
		suiOwner = evt.Sender.String()
	}
	if v, ok := payload["redeemer"].(string); ok && suiOwner == "" {
		suiOwner = v
	}

	sub := RedeemSubmission{
		SuiTxDigest:  evt.Id.TxDigest.String(),
		SuiOwner:     suiOwner,
		EthRecipient: ethRecipient,
		ChainID:      ChainIDEthereum,
		Asset:        "ETH",
		Token:        token,
		Amount:       amountDec,
	}
	handle(ctx, sub)
}

func (l *SuiBridgeRedeemListener) tokenFromEvent(evt suiclient.Event) string {
	if evt.Type == nil {
		return ""
	}
	if l.fEventType != nil && evt.Type.String() == l.fEventType.String() {
		return "f"
	}
	if l.xEventType != nil && evt.Type.String() == l.xEventType.String() {
		return "x"
	}
	return ""
}

func parseAmountDecimal(v any) (decimal.Decimal, error) {
	switch amt := v.(type) {
	case string:
		if amt == "" {
			return decimal.Zero, fmt.Errorf("empty amount")
		}
		bi, ok := new(big.Int).SetString(amt, 10)
		if !ok {
			return decimal.Zero, fmt.Errorf("invalid amount string %s", amt)
		}
		return decimal.NewFromBigInt(bi, -9), nil
	case float64:
		return decimal.NewFromFloat(amt).Div(decimal.New(1, 9)), nil
	case json.Number:
		return parseAmountDecimal(string(amt))
	default:
		return decimal.Zero, fmt.Errorf("unsupported amount type %T", v)
	}
}

func parseEthRecipient(v any) string {
	switch rec := v.(type) {
	case string:
		return strings.ToLower(strings.TrimSpace(rec))
	case []any:
		buf := make([]byte, 0, len(rec))
		for _, b := range rec {
			switch val := b.(type) {
			case float64:
				buf = append(buf, byte(val))
			case json.Number:
				i, _ := val.Int64()
				buf = append(buf, byte(i))
			}
		}
		if len(buf) > 0 {
			return "0x" + strings.ToLower(hex.EncodeToString(buf))
		}
	}
	return ""
}

func initSuiWebsocket(client *suiclient.ClientImpl, wsURL string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("connect sui websocket %s: %v", wsURL, r)
		}
	}()
	client.WithWebsocket(context.Background(), wsURL)
	return nil
}

func inferSuiWebsocketURL(rpc, network string) string {
	if rpc != "" {
		if u, err := url.Parse(rpc); err == nil {
			switch u.Scheme {
			case "https":
				u.Scheme = "wss"
			case "http":
				u.Scheme = "ws"
			case "ws", "wss":
			default:
				u.Scheme = ""
			}
			if u.Scheme != "" {
				return u.String()
			}
		}
	}

	switch strings.ToLower(strings.TrimSpace(network)) {
	case "testnet":
		return "wss://fullnode.testnet.sui.io"
	case "mainnet":
		return "wss://fullnode.mainnet.sui.io"
	default:
		return "ws://localhost:9000"
	}
}
