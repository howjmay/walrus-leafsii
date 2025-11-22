package crosschain

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	bcs "github.com/fardream/go-bcs/bcs"
	"github.com/pattonkan/sui-go/sui"
	"github.com/pattonkan/sui-go/sui/suiptb"
	suiclient "github.com/pattonkan/sui-go/suiclient"
	"github.com/pattonkan/sui-go/suisigner"
	suicrypto "github.com/pattonkan/sui-go/suisigner/suicrypto"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// SuiBridgeMinter mints bridged deposits onto Sui using the bridge_mint entrypoints.
// Enabled when LFS_ENABLE_BRIDGE_MINT is truthy and required env vars are provided.
type SuiBridgeMinter struct {
	cfg    bridgeMintConfig
	client *suiclient.ClientImpl
	signer *suisigner.Signer
	logger *zap.SugaredLogger
}

type bridgeMintConfig struct {
	rpc          string
	fTokenType   string
	xTokenType   string
	fTreasuryCap string
	xTreasuryCap string
	fMintAuth    string
	xMintAuth    string
}

// NewSuiBridgeMinterFromEnv returns a configured minter when enabled; otherwise nil.
func NewSuiBridgeMinterFromEnv(logger *zap.SugaredLogger) (*SuiBridgeMinter, error) {
	if !isTruthy(os.Getenv("LFS_ENABLE_BRIDGE_MINT")) {
		return nil, nil
	}

	cfg := bridgeMintConfig{
		rpc:          strings.TrimSpace(os.Getenv("LFS_SUI_RPC_URL")),
		fTokenType:   strings.TrimSpace(os.Getenv("LFS_SUI_FTOKEN_TYPE")),
		xTokenType:   strings.TrimSpace(os.Getenv("LFS_SUI_XTOKEN_TYPE")),
		fTreasuryCap: strings.TrimSpace(os.Getenv("LFS_SUI_FTOKEN_TREASURY_CAP")),
		xTreasuryCap: strings.TrimSpace(os.Getenv("LFS_SUI_XTOKEN_TREASURY_CAP")),
		fMintAuth:    strings.TrimSpace(os.Getenv("LFS_SUI_FTOKEN_AUTHORITY")),
		xMintAuth:    strings.TrimSpace(os.Getenv("LFS_SUI_XTOKEN_AUTHORITY")),
	}

	if cfg.rpc == "" || cfg.fTokenType == "" || cfg.xTokenType == "" ||
		cfg.fTreasuryCap == "" || cfg.xTreasuryCap == "" ||
		cfg.fMintAuth == "" || cfg.xMintAuth == "" {
		return nil, fmt.Errorf("bridge minter enabled but missing required env; need LFS_SUI_RPC_URL, LFS_SUI_FTOKEN_TYPE, LFS_SUI_XTOKEN_TYPE, LFS_SUI_FTOKEN_TREASURY_CAP, LFS_SUI_XTOKEN_TREASURY_CAP, LFS_SUI_FTOKEN_AUTHORITY, LFS_SUI_XTOKEN_AUTHORITY")
	}

	if !strings.Contains(cfg.fTokenType, "::ftoken::") || !strings.Contains(cfg.xTokenType, "::xtoken::") {
		return nil, fmt.Errorf("bridge minter requires ftoken/xtoken coin types (got %s / %s)", cfg.fTokenType, cfg.xTokenType)
	}

	mnemonic := strings.TrimSpace(os.Getenv("LFS_SUI_DEPLOY_MNEMONIC"))
	if mnemonic == "" {
		return nil, fmt.Errorf("bridge minter enabled but LFS_SUI_DEPLOY_MNEMONIC is empty")
	}

	signer, err := suisigner.NewSignerWithMnemonic(mnemonic, suicrypto.KeySchemeFlagEd25519)
	if err != nil {
		return nil, fmt.Errorf("build Sui signer: %w", err)
	}

	client := suiclient.NewClient(cfg.rpc)

	logger.Infow("Bridge mint handler enabled",
		"suiRpc", cfg.rpc,
		"fTokenType", cfg.fTokenType,
		"xTokenType", cfg.xTokenType,
	)

	return &SuiBridgeMinter{
		cfg:    cfg,
		client: client,
		signer: signer,
		logger: logger,
	}, nil
}

func (m *SuiBridgeMinter) Mint(ctx context.Context, payload BridgeMintContext) (*MintResult, error) {
	recipient, err := sui.AddressFromHex(payload.Submission.SuiOwner)
	if err != nil {
		return nil, fmt.Errorf("invalid Sui owner: %w", err)
	}

	mintF := payload.MintF
	mintX := payload.MintX
	if mintF == 0 && mintX == 0 {
		// Fallback to pre-existing behavior if split amounts were not provided.
		if amt, ok := deriveMintAmount(payload.NewShares); ok {
			mintF = amt
			mintX = amt
		}
	}
	if mintF == 0 && mintX == 0 {
		return nil, fmt.Errorf("derived zero mint amount from %s", payload.NewShares.String())
	}

	fPkg := parsePkg(m.cfg.fTokenType)
	xPkg := parsePkg(m.cfg.xTokenType)
	if fPkg == "" || xPkg == "" {
		return nil, fmt.Errorf("unable to parse package ids from f/x token types")
	}

	digests := []string{}

	if mintF > 0 {
		if digest, err := m.mintPackage(ctx, fPkg, "ftoken", m.cfg.fTreasuryCap, m.cfg.fMintAuth, mintF, *recipient); err != nil {
			return nil, fmt.Errorf("ftoken mint: %w", err)
		} else if digest != "" {
			digests = append(digests, digest)
		}
	}
	if mintX > 0 {
		if digest, err := m.mintPackage(ctx, xPkg, "xtoken", m.cfg.xTreasuryCap, m.cfg.xMintAuth, mintX, *recipient); err != nil {
			return nil, fmt.Errorf("xtoken mint: %w", err)
		} else if digest != "" {
			digests = append(digests, digest)
		}
	}

	return &MintResult{TxDigests: digests}, nil
}

func (m *SuiBridgeMinter) mintPackage(ctx context.Context, pkgHex, module, treasuryCap, authority string, amount uint64, recipient sui.Address) (string, error) {
	txCtx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()

	pkg := sui.MustPackageIdFromHex(pkgHex)

	treasuryObj, err := m.client.GetObject(txCtx, &suiclient.GetObjectRequest{
		ObjectId: sui.MustObjectIdFromHex(treasuryCap),
		Options:  &suiclient.SuiObjectDataOptions{ShowOwner: true},
	})
	if err != nil {
		return "", fmt.Errorf("fetch treasury cap: %w", err)
	}
	if treasuryObj == nil || treasuryObj.Data == nil || treasuryObj.Data.Ref() == nil {
		return "", fmt.Errorf("treasury cap not found: %s", treasuryCap)
	}
	ownerAddr := ownedAddress(treasuryObj.Data.Owner)
	if ownerAddr == nil || m.signer.Address == nil || *ownerAddr != *m.signer.Address {
		return "", fmt.Errorf("treasury cap must be owned by signer %s", m.signer.Address.String())
	}
	authArg, err := m.sharedArg(txCtx, authority, false)
	if err != nil {
		return "", fmt.Errorf("authority shared ref: %w", err)
	}

	coins, err := m.client.GetCoins(txCtx, &suiclient.GetCoinsRequest{Owner: m.signer.Address})
	if err != nil {
		return "", fmt.Errorf("get gas coins: %w", err)
	}
	if len(coins.Data) == 0 {
		return "", fmt.Errorf("no SUI coins available for gas; fund %s", m.signer.Address.String())
	}

	ptb := suiptb.NewTransactionDataTransactionBuilder()
	ptb.Command(suiptb.Command{
		MoveCall: &suiptb.ProgrammableMoveCall{
			Package:  pkg,
			Module:   module,
			Function: "bridge_mint",
			Arguments: []suiptb.Argument{
				ptb.MustObj(suiptb.ObjectArg{ImmOrOwnedObject: treasuryObj.Data.Ref()}),
				ptb.MustObj(authArg),
				ptb.MustPure(amount),
				ptb.MustPure(recipient),
			},
		},
	})

	pt := ptb.Finish()
	tx := suiptb.NewTransactionData(
		m.signer.Address,
		pt,
		[]*sui.ObjectRef{coins.Data[0].Ref()},
		10*suiclient.DefaultGasBudget,
		suiclient.DefaultGasPrice,
	)

	txBytes, err := bcs.Marshal(tx)
	if err != nil {
		return "", fmt.Errorf("marshal tx: %w", err)
	}

	resp, err := m.client.SignAndExecuteTransaction(
		txCtx,
		m.signer,
		txBytes,
		&suiclient.SuiTransactionBlockResponseOptions{ShowEffects: true},
	)
	if err != nil {
		return "", fmt.Errorf("execute tx: %w", err)
	}
	if resp == nil || resp.Effects == nil || !resp.Effects.Data.IsSuccess() {
		return "", fmt.Errorf("bridge mint transaction failed: %v", resp.Errors)
	}

	m.logger.Infow("Bridge mint succeeded",
		"module", module,
		"digest", resp.Digest,
		"recipient", recipient.String(),
		"amount", amount,
	)

	return resp.Digest.String(), nil
}

func (m *SuiBridgeMinter) sharedArg(ctx context.Context, id string, mutable bool) (suiptb.ObjectArg, error) {
	oid, err := sui.ObjectIdFromHex(id)
	if err != nil {
		return suiptb.ObjectArg{}, fmt.Errorf("parse object id: %w", err)
	}
	obj, err := m.client.GetObject(ctx, &suiclient.GetObjectRequest{
		ObjectId: oid,
		Options:  &suiclient.SuiObjectDataOptions{ShowOwner: true},
	})
	if err != nil {
		return suiptb.ObjectArg{}, fmt.Errorf("fetch object %s: %w", id, err)
	}
	ref := obj.Data.RefSharedObject()
	return suiptb.ObjectArg{
		SharedObject: &suiptb.SharedObjectArg{
			Id:                   ref.ObjectId,
			InitialSharedVersion: ref.Version,
			Mutable:              mutable,
		},
	}, nil
}

func deriveMintAmount(amount decimal.Decimal) (uint64, bool) {
	// Token decimals = 9, ETH wei = 1e18 → scale down by 1e9 (equivalent to amount * 1e9).
	mint := amount.Shift(9)
	if mint.LessThanOrEqual(decimal.Zero) {
		return 0, false
	}
	b := mint.BigInt()
	if b == nil || b.Sign() <= 0 {
		return 0, false
	}
	if !b.IsUint64() {
		return 0, false
	}
	return b.Uint64(), true
}

func parsePkg(coinType string) string {
	part := strings.SplitN(coinType, "::", 2)
	if len(part) == 0 {
		return ""
	}
	return strings.TrimSpace(part[0])
}

func ownedAddress(owner *suiclient.ObjectOwner) *sui.Address {
	if owner == nil || owner.ObjectOwnerInternal == nil {
		return nil
	}
	if owner.AddressOwner != nil {
		return owner.AddressOwner
	}
	if owner.SingleOwner != nil {
		return owner.SingleOwner
	}
	if owner.ObjectOwner != nil {
		return owner.ObjectOwner
	}
	return nil
}

func isTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
