package spaces

import (
	"context"
	"fmt"
	"io"
	"mime"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type Client struct {
	s3       *s3.Client
	bucket   string
	region   string
	cdnURL   string
	endpoint string
}

func NewClient(accessKey, secretKey, bucket, region, endpoint, cdnURL string) (*Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("spaces: load config: %w", err)
	}

	return &Client{
		s3: s3.NewFromConfig(cfg, func(o *s3.Options) {
			o.UsePathStyle = true
			o.BaseEndpoint = aws.String(endpoint)
		}),
		bucket:   bucket,
		region:   region,
		cdnURL:   cdnURL,
		endpoint: endpoint,
	}, nil
}

// Upload stores r under key in the bucket and returns the public URL.
func (c *Client) Upload(ctx context.Context, key string, r io.Reader, size int64) (string, error) {
	contentType := mime.TypeByExtension(filepath.Ext(key))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	// The system MIME database maps .webm → video/webm, but all WebM files
	// in this app come from the voice recorder (MediaRecorder API) and are
	// audio-only.  Serving them as video/webm causes <audio> elements to
	// receive a MIME-type mismatch and fire an error event instead of
	// loadedmetadata, leaving the play button permanently disabled.
	if contentType == "video/webm" {
		contentType = "audio/webm"
	}

	_, err := c.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(c.bucket),
		Key:           aws.String(key),
		Body:          r,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(contentType),
		ACL:           "public-read",
	})
	if err != nil {
		return "", fmt.Errorf("spaces: put object: %w", err)
	}

	return fmt.Sprintf("%s/%s", c.cdnURL, key), nil
}

// ConfigureCORS sets a CORS rule on the bucket allowing GET requests from the
// given origin. Called once at startup so audio files can be fetched via the
// Web Audio API from the browser. If origin is empty, the rule is skipped.
func (c *Client) ConfigureCORS(ctx context.Context, origin string) error {
	if origin == "" {
		return nil
	}
	_, err := c.s3.PutBucketCors(ctx, &s3.PutBucketCorsInput{
		Bucket: aws.String(c.bucket),
		CORSConfiguration: &types.CORSConfiguration{
			CORSRules: []types.CORSRule{
				{
					AllowedOrigins: []string{origin},
					AllowedMethods: []string{"GET"},
					AllowedHeaders: []string{"*"},
					MaxAgeSeconds:  aws.Int32(3600),
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("spaces: put bucket cors: %w", err)
	}
	return nil
}

// Delete removes an object from the bucket by key.
func (c *Client) Delete(ctx context.Context, key string) error {
	_, err := c.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	return err
}
