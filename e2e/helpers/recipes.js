import { createServer } from 'http';

// One-recipe community catalog served from 127.0.0.1, so specs do not depend
// on the live catalog. Pass indexURL as startServer's recipesIndexURL.
export const FIXTURE_RECIPE = {
  id: 'e2e-static-site',
  name: 'E2E Static Site',
  description: 'Fixture recipe served by the E2E suite.',
  compose: [
    'services:',
    '  web:',
    '    image: nginx:alpine',
    '    labels:',
    '      simpledeploy.endpoints.0.domain: "recipe-e2e.local"',
    '      simpledeploy.endpoints.0.port: "80"',
    '      simpledeploy.endpoints.0.tls: "off"',
    '',
  ].join('\n'),
  readme: '# E2E Static Site\n\nFixture README marker e2e-readme-7f3a.\n',
};

export async function startRecipesCatalog() {
  let base = '';
  const r = FIXTURE_RECIPE;
  const files = () => ({
    '/index.json': ['application/json', JSON.stringify({
      schema_version: 1,
      generated_at: new Date().toISOString(),
      recipes: [{
        id: r.id,
        name: r.name,
        category: 'web',
        description: r.description,
        tags: ['e2e'],
        author: 'e2e',
        compose_url: `${base}/recipes/${r.id}/compose.yml`,
        readme_url: `${base}/recipes/${r.id}/README.md`,
        schema_version: 1,
      }],
    })],
    [`/recipes/${r.id}/compose.yml`]: ['text/plain', r.compose],
    [`/recipes/${r.id}/README.md`]: ['text/plain', r.readme],
  });

  const server = createServer((req, res) => {
    const f = files()[req.url];
    if (!f) {
      res.writeHead(404);
      res.end();
      return;
    }
    res.writeHead(200, { 'Content-Type': f[0] });
    res.end(f[1]);
  });

  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  base = `http://127.0.0.1:${server.address().port}`;
  return {
    indexURL: `${base}/index.json`,
    stop: () => new Promise((resolve) => server.close(() => resolve())),
  };
}
