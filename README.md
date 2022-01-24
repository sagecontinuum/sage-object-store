# sage-object-store
API to access BLOB data collected from Sage nodes



URLs to request files from he API have to be in this format:
```console
curl localhost:8080/api/v1/data/<job_id>/<task_id>/<node_id>/<timestamp>-<filename>
```

The path `<job_id>/<task_id>/<node_id>/<timestamp>-<filename>` reflects how files are stored in the backend S3.


# Testing

```console
export TESTING=1 ; go test .
```