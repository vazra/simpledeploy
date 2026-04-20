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
        test: ['CMD-SHELL', 'pg_isready -U "$${POSTGRES_USER:-postgres}" -d "$${POSTGRES_DB:-postgres}"'],
        interval: '10s',
        timeout: '5s',
        retries: 5,
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
        test: ['CMD-SHELL', 'mysqladmin ping -h localhost -uroot -p"$$MYSQL_ROOT_PASSWORD" --silent'],
        interval: '10s',
        timeout: '5s',
        retries: 10,
        start_period: '60s',
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
        retries: 10,
        start_period: '60s',
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
        test: ['CMD-SHELL', "mongosh --quiet -u root -p changeme --authenticationDatabase admin --eval \"db.adminCommand('ping').ok\" | grep -q 1"],
        interval: '10s',
        timeout: '10s',
        retries: 10,
        start_period: '40s',
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
      healthcheck: {
        test: ['CMD-SHELL', 'curl -fsS http://localhost:5984/_up || exit 1'],
        interval: '15s',
        timeout: '5s',
        retries: 5,
        start_period: '30s',
      },
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
        test: ['CMD', 'redis-cli', 'ping'],
        interval: '10s',
        timeout: '5s',
        retries: 5,
        start_period: '10s',
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
        test: ['CMD', 'valkey-cli', 'ping'],
        interval: '10s',
        timeout: '5s',
        retries: 5,
        start_period: '10s',
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
      healthcheck: {
        test: ['CMD-SHELL', 'echo stats | nc -w 1 127.0.0.1 11211 | grep -q uptime'],
        interval: '10s',
        timeout: '5s',
        retries: 5,
        start_period: '10s',
      },
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
      healthcheck: {
        test: ['CMD-SHELL', 'wget --no-verbose --tries=1 --spider http://localhost:8123/ping || exit 1'],
        interval: '15s',
        timeout: '5s',
        retries: 5,
        start_period: '30s',
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
      healthcheck: {
        test: ['CMD-SHELL', 'wget -qO- http://localhost:7700/health || exit 1'],
        interval: '15s',
        timeout: '5s',
        retries: 5,
        start_period: '20s',
      },
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
      healthcheck: {
        test: ['CMD-SHELL', 'curl -fsS http://localhost:8108/health || exit 1'],
        interval: '15s',
        timeout: '5s',
        retries: 5,
        start_period: '20s',
      },
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
        test: ['CMD', 'rabbitmq-diagnostics', '-q', 'ping'],
        interval: '15s',
        timeout: '15s',
        retries: 10,
        start_period: '60s',
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
      command: '-js -sd /data -m 8222',
      volumes: ['natsdata:/data'],
      healthcheck: {
        test: ['CMD-SHELL', 'wget -qO- http://localhost:8222/healthz || exit 1'],
        interval: '15s',
        timeout: '5s',
        retries: 5,
        start_period: '15s',
      },
    },
  },

  // ---- Storage / files ----
  {
    id: 'minio',
    name: 'MinIO',
    icon: '🪣',
    description: 'S3-compatible object storage',
    config: {
      image: 'minio/minio:RELEASE.2024-10-13T13-34-11Z',
      environment: {
        MINIO_ROOT_USER: 'admin',
        MINIO_ROOT_PASSWORD: 'changeme-at-least-8-chars',
      },
      volumes: ['miniodata:/data'],
      command: 'server /data --console-address ":9001"',
      restart: 'unless-stopped',
      healthcheck: {
        test: ['CMD-SHELL', 'curl -fsS http://localhost:9000/minio/health/live || exit 1'],
        interval: '15s',
        timeout: '5s',
        retries: 5,
        start_period: '20s',
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
        retries: 5,
        start_period: '10s',
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
      healthcheck: {
        test: ['CMD', '/mailpit', 'readyz'],
        interval: '15s',
        timeout: '5s',
        retries: 5,
        start_period: '15s',
      },
    },
  },
  {
    id: 'adminer',
    name: 'Adminer',
    icon: '🛠️',
    description: 'Lightweight web UI for managing SQL databases',
    config: {
      image: 'adminer:4',
      restart: 'unless-stopped',
      healthcheck: {
        test: ['CMD-SHELL', 'php -r "exit(@fsockopen(\'localhost\',8080)?0:1);"'],
        interval: '15s',
        timeout: '5s',
        retries: 5,
        start_period: '15s',
      },
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
        retries: 5,
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
      // Default config disallows anonymous; provide a permissive default for quick-start.
      command: "sh -c \"printf 'listener 1883\\nallow_anonymous true\\n' > /mosquitto/config/mosquitto.conf && exec /docker-entrypoint.sh /usr/sbin/mosquitto -c /mosquitto/config/mosquitto.conf\"",
      volumes: [
        'mosquittodata:/mosquitto/data',
        'mosquittologs:/mosquitto/log',
      ],
      healthcheck: {
        test: ['CMD-SHELL', 'mosquitto_sub -h localhost -t healthcheck -E -i healthcheck-probe -W 2 || exit 1'],
        interval: '15s',
        timeout: '5s',
        retries: 5,
        start_period: '15s',
      },
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
      healthcheck: {
        test: ['CMD', 'extra/healthcheck'],
        interval: '30s',
        timeout: '10s',
        retries: 5,
        start_period: '60s',
      },
    },
  },
  {
    id: 'prometheus',
    name: 'Prometheus',
    icon: '🔥',
    description: 'Metrics collection and time-series database',
    config: {
      image: 'prom/prometheus:v2.55.1',
      restart: 'unless-stopped',
      user: '65534:65534',
      volumes: ['prometheusdata:/prometheus'],
      command: '--config.file=/etc/prometheus/prometheus.yml --storage.tsdb.path=/prometheus',
      healthcheck: {
        test: ['CMD', 'wget', '--no-verbose', '--tries=1', '--spider', 'http://localhost:9090/-/healthy'],
        interval: '15s',
        timeout: '5s',
        retries: 5,
        start_period: '20s',
      },
    },
  },
  {
    id: 'grafana',
    name: 'Grafana',
    icon: '📉',
    description: 'Dashboards and visualization for metrics',
    config: {
      image: 'grafana/grafana-oss:11.3.0',
      restart: 'unless-stopped',
      user: '472:472',
      volumes: ['grafanadata:/var/lib/grafana'],
      environment: {
        GF_SECURITY_ADMIN_PASSWORD: 'changeme',
      },
      healthcheck: {
        test: ['CMD-SHELL', 'wget -qO- http://localhost:3000/api/health | grep -q ok || exit 1'],
        interval: '15s',
        timeout: '5s',
        retries: 5,
        start_period: '30s',
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
      // Authelia requires a /config/configuration.yml before it will start.
      // Long start_period gives the user time to mount or generate one.
      healthcheck: {
        test: ['CMD-SHELL', 'wget -qO- http://localhost:9091/api/health || exit 1'],
        interval: '30s',
        timeout: '5s',
        retries: 3,
        start_period: '120s',
      },
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
      healthcheck: {
        test: ['CMD-SHELL', 'curl -fsS http://localhost:80/alive || exit 1'],
        interval: '30s',
        timeout: '5s',
        retries: 5,
        start_period: '30s',
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
      healthcheck: {
        test: ['CMD-SHELL', 'curl -fsS http://localhost:3000/api/healthz || exit 1'],
        interval: '30s',
        timeout: '5s',
        retries: 5,
        start_period: '60s',
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
      healthcheck: {
        test: ['CMD-SHELL', 'wget -qO- http://localhost:5678/healthz || exit 1'],
        interval: '15s',
        timeout: '5s',
        retries: 5,
        start_period: '60s',
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
      // Long start_period: umami waits on an external Postgres before becoming healthy.
      healthcheck: {
        test: ['CMD-SHELL', 'wget -qO- http://localhost:3000/api/heartbeat || exit 1'],
        interval: '30s',
        timeout: '5s',
        retries: 5,
        start_period: '120s',
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
      healthcheck: {
        test: ['CMD-SHELL', 'wget -qO- http://localhost:3000/ >/dev/null || exit 1'],
        interval: '30s',
        timeout: '5s',
        retries: 5,
        start_period: '30s',
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
      healthcheck: {
        test: ['CMD-SHELL', 'wget -qO- http://localhost:8090/api/health || exit 1'],
        interval: '15s',
        timeout: '5s',
        retries: 5,
        start_period: '20s',
      },
    },
  },
];
