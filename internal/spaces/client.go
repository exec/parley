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

// Upload stores r under key in the public-assets bucket with a public-read ACL
// and returns the CDN URL. Only use for objects safe for anonymous GET
// (avatars, user uploads, soundboard, audio).
func (c *Client) Upload(ctx context.Context, key string, r io.Reader, size int64) (string, error) {
	return c.put(ctx, key, r, size, true)
}

// UploadPrivate stores r under key with a private ACL. Returned string is the
// bucket-relative key — callers must use presigned URLs or an authenticated
// download path to serve it. Use for anything that must not be anonymously
// readable (e.g. database backups).
func (c *Client) UploadPrivate(ctx context.Context, key string, r io.Reader, size int64) (string, error) {
	return c.put(ctx, key, r, size, false)
}

func (c *Client) put(ctx context.Context, key string, r io.Reader, size int64, public bool) (string, error) {
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

	in := &s3.PutObjectInput{
		Bucket:        aws.String(c.bucket),
		Key:           aws.String(key),
		Body:          r,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(contentType),
	}
	if public {
		in.ACL = "public-read"
	} else {
		in.ACL = "private"
	}

	if _, err := c.s3.PutObject(ctx, in); err != nil {
		return "", fmt.Errorf("spaces: put object: %w", err)
	}

	if public {
		return fmt.Sprintf("%s/%s", c.cdnURL, key), nil
	}
	return key, nil
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
