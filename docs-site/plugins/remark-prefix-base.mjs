// Rewrites absolute internal links (e.g. "/start/quickstart/") in markdown
// and MDX content to be prefixed with the configured site base, so that
// hosting under a subpath like /simpledeploy/ does not produce 404s.
//
// Skips: external URLs, protocol-relative URLs, fragment-only links,
// already-prefixed paths, and asset paths Astro handles itself (_astro, etc).

const DEFAULT_SKIP_PREFIXES = ["/_astro/", "/@", "/api/"];

function shouldRewrite(url, base) {
  if (typeof url !== "string" || url.length === 0) return false;
  if (!url.startsWith("/")) return false;
  if (url.startsWith("//")) return false;
  if (url.startsWith(base + "/") || url === base) return false;
  for (const p of DEFAULT_SKIP_PREFIXES) if (url.startsWith(p)) return false;
  return true;
}

function rewrite(url, base) {
  return base + url;
}

function visitNode(node, base) {
  if (!node || typeof node !== "object") return;

  if ((node.type === "link" || node.type === "image") && shouldRewrite(node.url, base)) {
    node.url = rewrite(node.url, base);
  }

  if (node.type === "mdxJsxFlowElement" || node.type === "mdxJsxTextElement") {
    if (Array.isArray(node.attributes)) {
      for (const attr of node.attributes) {
        if (
          attr &&
          attr.type === "mdxJsxAttribute" &&
          (attr.name === "href" || attr.name === "src" || attr.name === "link") &&
          typeof attr.value === "string" &&
          shouldRewrite(attr.value, base)
        ) {
          attr.value = rewrite(attr.value, base);
        }
      }
    }
  }

  if (Array.isArray(node.children)) {
    for (const child of node.children) visitNode(child, base);
  }
}

export default function remarkPrefixBase(options = {}) {
  const base = (options.base || "").replace(/\/$/, "");
  if (!base) return () => {};
  return (tree) => {
    visitNode(tree, base);
  };
}
