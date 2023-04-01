package scrape

// Documentation for interacting with aws-sdk-go-v2 https://aws.github.io/aws-sdk-go-v2/docs/getting-started/
import (
	"context"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	sq "github.com/aws/aws-sdk-go-v2/service/servicequotas"
	"github.com/emylincon/aws_quota_exporter/pkg"
)

// Scraper struct
type Scraper struct {
	cfg aws.Config
}

// NewScraper creates a new Scraper
func NewScraper() (*Scraper, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO()) // config.WithRegion("us-west-2")
	if err != nil {
		return &Scraper{}, err
	}

	return &Scraper{cfg: cfg}, nil
}

// CreateScraper Scrape Quotas from AWS
func (s *Scraper) CreateScraper(regions []string, serviceCode string) func() ([]*pkg.PrometheusMetric, error) {
	return func() ([]*pkg.PrometheusMetric, error) {
		ctx := context.Background()

		sclient := sq.NewFromConfig(s.cfg)
		input := sq.ListServiceQuotasInput{ServiceCode: &serviceCode}
		metricList := []*pkg.PrometheusMetric{}

		for _, region := range regions {
			metrics, err := getServiceQuotas(ctx, region, &input, sclient)
			if err != nil {
				fmt.Printf("Failed to get service quotas: %v\n", err)
				return nil, err
			}

			metricList = append(metricList, metrics...)
		}
		return metricList, nil

	}

}

// Transform to prometheus format
func Transform(quotas *sq.ListServiceQuotasOutput, defaultQuotas *sq.ListAWSDefaultServiceQuotasOutput, region string) ([]*pkg.PrometheusMetric, error) {
	metrics := []*pkg.PrometheusMetric{}
	check := map[string]bool{}
	for _, v := range quotas.Quotas {
		metricName := createMetricName(*v.ServiceCode, *v.QuotaName)
		metric := &pkg.PrometheusMetric{
			Name:   metricName,
			Value:  *v.Value,
			Labels: map[string]string{"adjustable": strconv.FormatBool(v.Adjustable), "global_quota": strconv.FormatBool(v.GlobalQuota), "unit": *v.Unit, "region": region},
			Desc:   *v.QuotaName,
		}
		metrics = append(metrics, metric)
		check[metricName] = true
	}
	for _, d := range defaultQuotas.Quotas {
		metricName := createMetricName(*d.ServiceCode, *d.QuotaName)
		if _, ok := check[metricName]; !ok {
			metric := &pkg.PrometheusMetric{
				Name:   metricName,
				Value:  *d.Value,
				Labels: map[string]string{"adjustable": strconv.FormatBool(d.Adjustable), "global_quota": strconv.FormatBool(d.GlobalQuota), "unit": *d.Unit, "region": region},
				Desc:   *d.QuotaName,
			}
			metrics = append(metrics, metric)
		}
	}
	return metrics, nil
}

func createMetricName(serviceCode, quotaName string) string {
	return fmt.Sprintf("aws_quota_%s_%s", serviceCode, pkg.PromString(quotaName))
}

func getServiceQuotas(ctx context.Context, region string, sqInput *sq.ListServiceQuotasInput, client *sq.Client) ([]*pkg.PrometheusMetric, error) {
	opts := func(o *sq.Options) { o.Region = region }

	// Get applied Quotas
	r, err := client.ListServiceQuotas(ctx, sqInput, opts)
	if err != nil {
		return nil, err
	}

	// Get default Quotas
	d, err := client.ListAWSDefaultServiceQuotas(ctx, &sq.ListAWSDefaultServiceQuotasInput{ServiceCode: sqInput.ServiceCode}, opts)
	if err != nil {
		return nil, err
	}
	return Transform(r, d, region)
}
