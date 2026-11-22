package domain

import "context"

type JobQueue interface {
	Enqueue(ctx context.Context, params ScrapeParams) (int64, error)
	Get(ctx context.Context, id int64) (JobRecord, error)
	List(ctx context.Context, filter JobListFilter) (JobPage, error)
	Cancel(ctx context.Context, id int64) error
	EnqueuePurge(ctx context.Context, id int64) error
}

type ResultRepository interface {
	Save(ctx context.Context, result ScrapeResult) error
	FindByJobID(ctx context.Context, id int64) (ScrapeResult, error)
	CountByJobIDs(ctx context.Context, ids []int64) (map[int64]int, error)
	DeleteByJobID(ctx context.Context, id int64) (int64, error)
	DeleteExpired(ctx context.Context) (int64, error)
}

type JobRepository interface {
	Delete(ctx context.Context, id int64) (int64, error)
}

type JobIDCodec interface {
	Encode(id int64) string
	Decode(publicID string) (int64, error)
}

type Scraper interface {
	Register(job ScrapeJob) <-chan FlushResult
	Submit(ctx context.Context, job ScrapeJob) (stop func(), err error)
	ForceFlush(job ScrapeJob)
	JobDone()
}

type Geocoder interface {
	GeocodeCity(ctx context.Context, city, uf string) (GeoPoint, error)
	GeocodeAddress(ctx context.Context, address Address, city, uf string) (GeoPoint, error)
}
