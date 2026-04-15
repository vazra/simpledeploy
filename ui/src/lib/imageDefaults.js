const defaults = {
  postgres: {
    environment: {
      POSTGRES_PASSWORD: 'changeme',
      POSTGRES_USER: 'postgres',
      POSTGRES_DB: 'app',
    },
    volumes: ['pgdata:/var/lib/postgresql/data'],
    healthcheck: {
      test: ['CMD-SHELL', 'pg_isready -U postgres'],
      interval: '10s',
      timeout: '5s',
      retries: 3,
      start_period: '30s',
    },
  },
  mysql: {
    environment: {
      MYSQL_ROOT_PASSWORD: 'changeme',
      MYSQL_DATABASE: 'app',
      MYSQL_USER: 'app',
      MYSQL_PASSWORD: 'changeme',
    },
    volumes: ['mysqldata:/var/lib/mysql'],
    healthcheck: {
      test: ['CMD-SHELL', 'mysqladmin ping -h localhost'],
      interval: '10s',
      timeout: '5s',
      retries: 3,
      start_period: '30s',
    },
  },
  mariadb: {
    environment: {
      MARIADB_ROOT_PASSWORD: 'changeme',
      MARIADB_DATABASE: 'app',
      MARIADB_USER: 'app',
      MARIADB_PASSWORD: 'changeme',
    },
    volumes: ['mariadbdata:/var/lib/mysql'],
    healthcheck: {
      test: ['CMD-SHELL', 'healthcheck.sh --connect --innodb_initialized'],
      interval: '10s',
      timeout: '5s',
      retries: 3,
      start_period: '30s',
    },
  },
  redis: {
    volumes: ['redisdata:/data'],
    command: 'redis-server --appendonly yes',
    healthcheck: {
      test: ['CMD-SHELL', 'redis-cli ping'],
      interval: '10s',
      timeout: '5s',
      retries: 3,
      start_period: '30s',
    },
  },
  mongo: {
    environment: {
      MONGO_INITDB_ROOT_USERNAME: 'root',
      MONGO_INITDB_ROOT_PASSWORD: 'changeme',
    },
    volumes: ['mongodata:/data/db'],
    healthcheck: {
      test: ['CMD-SHELL', "mongosh --eval \"db.adminCommand('ping')\""],
      interval: '10s',
      timeout: '5s',
      retries: 3,
      start_period: '30s',
    },
  },
  rabbitmq: {
    environment: {
      RABBITMQ_DEFAULT_USER: 'guest',
      RABBITMQ_DEFAULT_PASS: 'changeme',
    },
    volumes: ['rabbitmqdata:/var/lib/rabbitmq'],
    ports: ['15672:15672'],
    healthcheck: {
      test: ['CMD-SHELL', 'rabbitmq-diagnostics -q ping'],
      interval: '10s',
      timeout: '5s',
      retries: 3,
      start_period: '30s',
    },
  },
  nginx: {
    ports: ['80:80'],
  },
};

const healthcheckCommands = {
  postgres: 'pg_isready -U postgres',
  mysql: 'mysqladmin ping -h localhost',
  mariadb: 'healthcheck.sh --connect --innodb_initialized',
  redis: 'redis-cli ping',
  mongo: "mongosh --eval \"db.adminCommand('ping')\"",
  rabbitmq: 'rabbitmq-diagnostics -q ping',
};

const patterns = ['postgres', 'mariadb', 'mysql', 'redis', 'rabbitmq', 'nginx', 'mongo'];

function matchImage(imageName) {
  const base = imageName.split(':')[0].toLowerCase();
  // check mariadb before mysql since mariadb contains no 'mysql' but order matters
  // mongo must match both 'mongo' and 'mongodb'
  for (const pattern of patterns) {
    if (base.includes(pattern)) return pattern;
  }
  return null;
}

export function getImageDefaults(imageName) {
  if (!imageName) return null;
  const match = matchImage(imageName);
  if (!match) return null;
  // mongodb -> mongo
  const key = match === 'mongodb' ? 'mongo' : match;
  return defaults[key] || null;
}

export function getHealthcheckSuggestion(imageName) {
  if (!imageName) return null;
  const match = matchImage(imageName);
  if (!match) return null;
  const key = match === 'mongodb' ? 'mongo' : match;
  return healthcheckCommands[key] || null;
}
