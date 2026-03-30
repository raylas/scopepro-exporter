# scopepro-exporter

Prometheus exporter for [Transcend ScopePro CLI](https://us.transcend-info.com/manual/ScopeProCLI/EN/) metrics — exposes S.M.A.R.T attributes, health percentage, and drive identification for Transcend embedded SSDs and SD cards.

## Usage

```
scopepro-exporter -devices /dev/sda,/dev/nvme0n1
```

The exporter requires `scopepro` to be installed and accessible. It must run with sufficient privileges (typically root) to execute `scopepro`.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-addr` | `:9993` | Server bind address |
| `-devices` | *(required)* | Comma-separated device paths to monitor |
| `-log-level` | `info` | Log level (debug, info, warn, error) |
| `-namespace` | `scopepro` | Metric namespace prefix |
| `-scopepro-path` | `scopepro` | Path to the scopepro binary |

## Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `scopepro_drive_info` | gauge | Drive identity (labels: device, type, model, firmware, serial, interface, manufacturer, product, revision) |
| `scopepro_health_percentage` | gauge | Drive health percentage |
| `scopepro_smart_*` | gauge | S.M.A.R.T attributes (one metric per attribute, label: device) |
| `scopepro_scrape_errors_total` | counter | Scrape errors per device |
| `scopepro_build_info` | gauge | Build information (label: version) |

## Prometheus Scrape Config

```yaml
scrape_configs:
  - job_name: scopepro
    static_configs:
      - targets: ['localhost:9993']
```

## Building

```
nix build
```

The binary is output to `result/bin/scopepro-exporter`.

### Docker image (Linux only)

```
nix build .#image
docker load < result
```

### Without Nix

```
go build -o scopepro-exporter ./cmd/scopepro-exporter
```

## Docker Deployment

The Nix-built image contains only the exporter binary. Since the exporter shells out to `scopepro`, you'll need an image that includes the [ScopePro CLI](https://us.transcend-info.com/manual/ScopeProCLI/EN/). Here's a sample `Dockerfile` that combines the exporter from ghcr.io with the CLI from Transcend's Ubuntu PPA:

```dockerfile
FROM ghcr.io/raylas/scopepro-exporter:latest AS exporter

FROM ubuntu:24.04
RUN apt-get update \
    && apt-get install -y --no-install-recommends software-properties-common \
    && add-apt-repository ppa:transcend-rd/scopepro-cli \
    && apt-get update \
    && apt-get install -y --no-install-recommends scopepro-cli \
    && apt-get purge -y software-properties-common \
    && apt-get autoremove -y \
    && rm -rf /var/lib/apt/lists/*
COPY --from=exporter /bin/scopepro-exporter /usr/local/bin/scopepro-exporter
ENTRYPOINT ["scopepro-exporter"]
```

Build and run with device access:

```
docker build -t scopepro-exporter-full .
docker run --privileged -p 9993:9993 scopepro-exporter-full -devices /dev/sda
```
