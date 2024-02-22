# Environment Variables Required

To properly configure the application, ensure the following environment variables are set:

- `ZINCSEARCH_FILES_DIR`: Directory where Emails files are located.
- `ZINCSEARCH_INDEX`: Name of the index for ZincSearch.
- `ZINC_FIRST_ADMIN_USER`: Username for the first admin user in ZincSearch.
- `ZINC_FIRST_ADMIN_PASSWORD`: Password for the first admin user in ZincSearch.
- `ZINCSEARCH_IP`: IP address of the ZincSearch host.
- `ZINCSEARCH_PORT`: Port number of the ZincSearch host.

These environment variables are necessary for configuring the application's behavior. Make sure they are correctly set to ensure proper functionality.

## Run project and generate profiling

Run project with terraform

```bash
go run .
go tool pprof -png mem_profile.pprof > memprofile.png
go tool pprof -png cpu_profile.pprof > cpuprofile.png
```