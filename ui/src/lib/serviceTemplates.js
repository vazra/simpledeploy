// Curated quick-start service templates. Each renders as a preconfigured
// compose service (image, env, volumes, healthcheck). Users are expected to
// review and tweak before deploying. Use stable, well-known public images so
// the e2e manifest check (tests/00-template-images.spec.js) passes.
export const serviceTemplates = [
  // ---- Relational databases ----
  {
    id: 'postgresql',
    name: 'PostgreSQL',
    icon: '🐘',
    description: 'Reliable open-source relational database',
    config: {
      image: 'postgres:16-alpine',
      environment: {
        POSTGRES_PASSWORD: 'changeme',
        POSTGRES_USER: 'postgres',
        POSTGRES_DB: 'app',
      },
      volumes: ['pgdata:/var/lib/postgresql/data'],
      restart: 'unless-stopped',
      healthcheck: {
        test: ['CMD-SHELL', 'pg_isready -U postgres'],
        interval: '10s',
        timeout: '5s',
        retries: 3,
        start_period: '30s',
      },
    },
  },
  {
    id: 'mysql',
    name: 'MySQL',
    icon: '🐬',
    description: 'Popular open-source relational database',
    config: {
      image: 'mysql:8',
      environment: {
        MYSQL_ROOT_PASSWORD: 'changeme',
        MYSQL_DATABASE: 'app',
        MYSQL_USER: 'app',
        MYSQL_PASSWORD: 'changeme',
      },
      volumes: ['mysqldata:/var/lib/mysql'],
      restart: 'unless-stopped',
      healthcheck: {
        test: ['CMD-SHELL', 'mysqladmin ping -h localhost'],
        interval: '10s',
        timeout: '5s',
        retries: 3,
        start_period: '30s',
      },
    },
  },
  {
    id: 'mariadb',
    name: 'MariaDB',
    icon: '🦭',
    description: 'Community-developed MySQL fork',
    config: {
      image: 'mariadb:11',
      environment: {
        MARIADB_ROOT_PASSWORD: 'changeme',
        MARIADB_DATABASE: 'app',
        MARIADB_USER: 'app',
        MARIADB_PASSWORD: 'changeme',
      },
      volumes: ['mariadbdata:/var/lib/mysql'],
      restart: 'unless-stopped',
      healthcheck: {
        test: ['CMD-SHELL', 'healthcheck.sh --connect --innodb_initialized'],
        interval: '10s',
        timeout: '5s',
        retries: 3,
        start_period: '30s',
      },
    },
  },

  // ---- Document / NoSQL ----
  {
    id: 'mongodb',
    name: 'MongoDB',
    icon: '🍃',
    description: 'Document-oriented NoSQL database',
    config: {
      image: 'mongo:7',
      environment: {
        MONGO_INITDB_ROOT_USERNAME: 'root',
        MONGO_INITDB_ROOT_PASSWORD: 'changeme',
      },
      volumes: ['mongodata:/data/db'],
      restart: 'unless-stopped',
      healthcheck: {
        test: ['CMD-SHELL', "mongosh --quiet --eval \"db.adminCommand('ping').ok\" | grep -q 1"],
        interval: '10s',
        timeout: '5s',
        retries: 3,
        start_period: '30s',
      },
    },
  },
  {
    id: 'couchdb',
    name: 'CouchDB',
    icon: '🛋️',
    description: 'Document database with HTTP API and replication',
    config: {
      image: 'couchdb:3',
      environment: {
        COUCHDB_USER: 'admin',
        COUCHDB_PASSWORD: 'changeme',
      },
      volumes: ['couchdbdata:/opt/couchdb/data'],
      restart: 'unless-stopped',
    },
  },

  // ---- Key-value / cache ----
  {
    id: 'redis',
    name: 'Redis',
    icon: '🔴',
    description: 'In-memory data store and cache',
    config: {
      image: 'redis:7-alpine',
      volumes: ['redisdata:/data'],
      restart: 'unless-stopped',
      command: 'redis-server --appendonly yes --save 60 1',
      healthcheck: {
        test: ['CMD-SHELL', 'redis-cli ping'],
        interval: '10s',
        timeout: '5s',
        retries: 3,
        start_period: '30s',
      },
    },
  },
  {
    id: 'valkey',
    name: 'Valkey',
    icon: '🗝️',
    description: 'Open-source Redis-compatible key-value store',
    config: {
      image: 'valkey/valkey:8-alpine',
      volumes: ['valkeydata:/data'],
      restart: 'unless-stopped',
      command: 'valkey-server --appendonly yes --save 60 1',
      healthcheck: {
        test: ['CMD-SHELL', 'valkey-cli ping'],
        interval: '10s',
        timeout: '5s',
        retries: 3,
        start_period: '30s',
      },
    },
  },
  {
    id: 'memcached',
    name: 'Memcached',
    icon: '🧠',
    description: 'High-performance distributed memory cache',
    config: {
      image: 'memcached:1.6-alpine',
      restart: 'unless-stopped',
      command: 'memcached -m 256',
    },
  },

  // ---- Analytics / search ----
  {
    id: 'clickhouse',
    name: 'ClickHouse',
    icon: '📊',
    description: 'Fast columnar OLAP database for analytics',
    config: {
      image: 'clickhouse/clickhouse-server:24-alpine',
      environment: {
        CLICKHOUSE_USER: 'default',
        CLICKHOUSE_PASSWORD: 'changeme',
        CLICKHOUSE_DB: 'app',
      },
      volumes: ['clickhousedata:/var/lib/clickhouse'],
      restart: 'unless-stopped',
      ulimits: {
        nofile: { soft: 262144, hard: 262144 },
      },
    },
  },
  {
    id: 'meilisearch',
    name: 'Meilisearch',
    icon: '🔎',
    description: 'Lightning-fast typo-tolerant search engine',
    config: {
      image: 'getmeili/meilisearch:v1.11',
      environment: {
        MEILI_MASTER_KEY: 'changeme-at-least-16-chars',
        MEILI_ENV: 'production',
      },
      volumes: ['meilidata:/meili_data'],
      restart: 'unless-stopped',
    },
  },
  {
    id: 'typesense',
    name: 'Typesense',
    icon: '🔠',
    description: 'Open-source, typo-tolerant search API',
    config: {
      image: 'typesense/typesense:27.1',
      environment: {
        TYPESENSE_API_KEY: 'changeme',
        TYPESENSE_DATA_DIR: '/data',
      },
      volumes: ['typesensedata:/data'],
      restart: 'unless-stopped',
    },
  },

  // ---- Messaging / queues ----
  {
    id: 'rabbitmq',
    name: 'RabbitMQ',
    icon: '🐇',
    description: 'Message broker with management UI',
    config: {
      image: 'rabbitmq:3-management-alpine',
      environment: {
        RABBITMQ_DEFAULT_USER: 'guest',
        RABBITMQ_DEFAULT_PASS: 'changeme',
      },
      volumes: ['rabbitmqdata:/var/lib/rabbitmq'],
      ports: ['15672:15672'],
      restart: 'unless-stopped',
      healthcheck: {
        test: ['CMD-SHELL', 'rabbitmq-diagnostics -q ping'],
        interval: '10s',
        timeout: '5s',
        retries: 3,
        start_period: '30s',
      },
    },
  },
  {
    id: 'nats',
    name: 'NATS',
    icon: '📨',
    description: 'Lightweight cloud-native messaging system',
    config: {
      image: 'nats:2-alpine',
      restart: 'unless-stopped',
      command: '-js -sd /data',
      volumes: ['natsdata:/data'],
    },
  },

  // ---- Storage / files ----
  {
    id: 'minio',
    name: 'MinIO',
    icon: '🪣',
    description: 'S3-compatible object storage',
    config: {
      image: 'minio/minio:latest',
      environment: {
        MINIO_ROOT_USER: 'admin',
        MINIO_ROOT_PASSWORD: 'changeme-at-least-8-chars',
      },
      volumes: ['miniodata:/data'],
      command: 'server /data --console-address ":9001"',
      restart: 'unless-stopped',
      healthcheck: {
        test: ['CMD-SHELL', 'mc ready local || curl -f http://localhost:9000/minio/health/live'],
        interval: '15s',
        timeout: '5s',
        retries: 3,
        start_period: '30s',
      },
    },
  },

  // ---- Web / networking ----
  {
    id: 'nginx',
    name: 'Nginx',
    icon: '🌐',
    description: 'Lightweight web server and reverse proxy',
    config: {
      image: 'nginx:alpine',
      restart: 'unless-stopped',
      healthcheck: {
        test: ['CMD-SHELL', 'wget -qO- http://localhost/ >/dev/null || exit 1'],
        interval: '15s',
        timeout: '5s',
        retries: 3,
        start_period: '20s',
      },
    },
  },

  // ---- Dev utilities ----
  {
    id: 'mailpit',
    name: 'Mailpit',
    icon: '📬',
    description: 'SMTP server + web UI for catching dev emails',
    config: {
      image: 'axllent/mailpit:latest',
      restart: 'unless-stopped',
      volumes: ['mailpitdata:/data'],
      environment: {
        MP_MAX_MESSAGES: '5000',
        MP_DATABASE: '/data/mailpit.db',
        MP_SMTP_AUTH_ACCEPT_ANY: '1',
        MP_SMTP_AUTH_ALLOW_INSECURE: '1',
      },
    },
  },
  {
    id: 'adminer',
    name: 'Adminer',
    icon: '🛠️',
    description: 'Lightweight web UI for managing SQL databases',
    config: {
      image: 'adminer:latest',
      restart: 'unless-stopped',
    },
  },

  // ---- Memory-efficient KV alt ----
  {
    id: 'dragonfly',
    name: 'Dragonfly',
    icon: '🐲',
    description: 'Memory-efficient Redis/Memcached-compatible store',
    config: {
      image: 'docker.dragonflydb.io/dragonflydb/dragonfly:latest',
      restart: 'unless-stopped',
      volumes: ['dragonflydata:/data'],
      ulimits: {
        memlock: -1,
      },
      healthcheck: {
        test: ['CMD', 'redis-cli', 'ping'],
        interval: '10s',
        timeout: '5s',
        retries: 3,
        start_period: '20s',
      },
    },
  },

  // ---- IoT / messaging ----
  {
    id: 'mosquitto',
    name: 'Mosquitto',
    icon: '🦟',
    description: 'Lightweight MQTT broker for IoT',
    config: {
      image: 'eclipse-mosquitto:2',
      restart: 'unless-stopped',
      volumes: [
        'mosquittodata:/mosquitto/data',
        'mosquittologs:/mosquitto/log',
      ],
    },
  },

  // ---- Monitoring ----
  {
    id: 'uptime-kuma',
    name: 'Uptime Kuma',
    icon: '📈',
    description: 'Self-hosted uptime monitor with alerting',
    config: {
      image: 'louislam/uptime-kuma:1',
      restart: 'unless-stopped',
      volumes: ['uptimekumadata:/app/data'],
    },
  },
  {
    id: 'prometheus',
    name: 'Prometheus',
    icon: '🔥',
    description: 'Metrics collection and time-series database',
    config: {
      image: 'prom/prometheus:latest',
      restart: 'unless-stopped',
      volumes: ['prometheusdata:/prometheus'],
      command: '--config.file=/etc/prometheus/prometheus.yml --storage.tsdb.path=/prometheus',
    },
  },
  {
    id: 'grafana',
    name: 'Grafana',
    icon: '📉',
    description: 'Dashboards and visualization for metrics',
    config: {
      image: 'grafana/grafana-oss:latest',
      restart: 'unless-stopped',
      volumes: ['grafanadata:/var/lib/grafana'],
      environment: {
        GF_SECURITY_ADMIN_PASSWORD: 'changeme',
      },
    },
  },

  // ---- Auth / secrets ----
  {
    id: 'authelia',
    name: 'Authelia',
    icon: '🛡️',
    description: 'Lightweight SSO and 2FA portal (~30MB)',
    config: {
      image: 'authelia/authelia:4',
      restart: 'unless-stopped',
      volumes: ['autheliaconfig:/config'],
    },
  },
  {
    id: 'vaultwarden',
    name: 'Vaultwarden',
    icon: '🔐',
    description: 'Rust-based Bitwarden-compatible password manager',
    config: {
      image: 'vaultwarden/server:latest',
      restart: 'unless-stopped',
      volumes: ['vaultwardendata:/data'],
      environment: {
        ADMIN_TOKEN: 'changeme-long-random-token',
        SIGNUPS_ALLOWED: 'false',
      },
    },
  },

  // ---- Self-hosted apps ----
  {
    id: 'gitea',
    name: 'Gitea',
    icon: '🍵',
    description: 'Lightweight self-hosted Git service',
    config: {
      image: 'gitea/gitea:1',
      restart: 'unless-stopped',
      volumes: [
        'giteadata:/data',
        '/etc/timezone:/etc/timezone:ro',
        '/etc/localtime:/etc/localtime:ro',
      ],
      environment: {
        USER_UID: '1000',
        USER_GID: '1000',
      },
    },
  },
  {
    id: 'n8n',
    name: 'n8n',
    icon: '🔗',
    description: 'Workflow automation with visual editor',
    config: {
      image: 'n8nio/n8n:latest',
      restart: 'unless-stopped',
      volumes: ['n8ndata:/home/node/.n8n'],
      environment: {
        N8N_HOST: 'localhost',
        GENERIC_TIMEZONE: 'UTC',
      },
    },
  },
  {
    id: 'umami',
    name: 'Umami',
    icon: '📊',
    description: 'Privacy-focused web analytics (needs Postgres)',
    config: {
      image: 'ghcr.io/umami-software/umami:postgresql-latest',
      restart: 'unless-stopped',
      environment: {
        DATABASE_URL: 'postgresql://postgres:changeme@postgresql:5432/umami',
        DATABASE_TYPE: 'postgresql',
        APP_SECRET: 'changeme-random-string',
      },
    },
  },
  {
    id: 'homepage',
    name: 'Homepage',
    icon: '🏠',
    description: 'Static YAML-driven services dashboard',
    config: {
      image: 'ghcr.io/gethomepage/homepage:latest',
      restart: 'unless-stopped',
      volumes: ['homepageconfig:/app/config'],
      environment: {
        HOMEPAGE_ALLOWED_HOSTS: '*',
      },
    },
  },
  {
    id: 'pocketbase',
    name: 'PocketBase',
    icon: '💼',
    description: 'Single-binary SQLite-backed BaaS (~20MB)',
    config: {
      image: 'ghcr.io/muchobien/pocketbase:latest',
      restart: 'unless-stopped',
      volumes: ['pocketbasedata:/pb_data'],
    },
  },
];
