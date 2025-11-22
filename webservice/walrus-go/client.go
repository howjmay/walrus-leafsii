package walrus

import (
	"crypto/sha256"
	"fmt"
)

// StoreOptions mirrors the options used by callers; only the referenced fields are modeled here.
type StoreOptions struct {
	Epochs       int
	SendObjectTo string
}

// Client is a minimal stub client that satisfies the API used in tests.
type Client struct {
	publisherURLs []string
}

// ClientOption configures the stub client.
type ClientOption func(*Client)

// WithPublisherURLs records custom publisher URLs; the stub does not dial them.
func WithPublisherURLs(urls []string) ClientOption {
	return func(c *Client) {
		c.publisherURLs = append([]string{}, urls...)
	}
}

// NewClient constructs a stub Walrus client.
func NewClient(opts ...ClientOption) *Client {
	c := &Client{}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	return c
}

// BlobResponse matches the shape returned by Store.
type BlobResponse struct {
	BlobID string `json:"blobId"`
}

// StoreResponse represents a stored blob reference.
type StoreResponse struct {
	Blob BlobResponse `json:"blob"`
}

// Store returns a deterministic blob ID derived from the payload; the stub does not persist data.
func (c *Client) Store(data []byte, opts *StoreOptions) (*StoreResponse, error) {
	_ = opts // opts are ignored in the stub
	sum := sha256.Sum256(data)
	blobID := fmt.Sprintf("walrus-%x", sum[:8])
	return &StoreResponse{Blob: BlobResponse{BlobID: blobID}}, nil
}

// NormalizeBlobResponse is a no-op in the stub implementation.
func (r *StoreResponse) NormalizeBlobResponse() {
}
