package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	createSageBucketErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sage_bucket_creation_errors_total",
			Help: "Number of errors during the sage bucket creation",
		},
		[]string{"error"},
	)
	bucketCounter = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "bucket_creation_total",
			Help: "Number of sage bucket creations",
		},
	)
	fileUploadCounter = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "file_upload_total",
			Help: "Number of file uploads",
		},
	)
	fileUploadByteSize = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "file_upload_byte_size_total",
			Help: "the number of bytes uploaded",
		},
	)
	fileDownloadByteSize = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "file_download_byte_size_total",
			Help: "the number of bytes downloaded",
		},
	)
)
