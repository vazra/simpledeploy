// @ts-check
import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";
import svelte from "@astrojs/svelte";
import starlightOpenAPI, { openAPISidebarGroups } from "starlight-openapi";
import starlightBlog from "starlight-blog";
import starlightLinksValidator from "starlight-links-validator";
import starlightImageZoom from "starlight-image-zoom";

const githubRepo = "https://github.com/vazra/simpledeploy";

export default defineConfig({
  site: "https://vazra.github.io",
  base: "/simpledeploy",
  integrations: [
    svelte(),
    starlight({
      title: "SimpleDeploy",
      description:
        "Single binary that deploys Docker Compose apps with HTTPS, backups, alerts, metrics.",
      logo: {
        src: "./src/assets/logo.svg",
        replacesTitle: false,
      },
      favicon: "/favicon.svg",
      social: [
        { icon: "github", label: "GitHub", href: githubRepo },
      ],
      editLink: {
        baseUrl: `${githubRepo}/edit/main/docs-site/`,
      },
      customCss: ["./src/styles/custom.css"],
      components: {
        SocialIcons: "./src/components/SocialIconsWithStar.astro",
      },
      plugins: [
        starlightImageZoom(),
        starlightBlog({
          title: "Blog",
          postCount: 10,
        }),
        starlightLinksValidator({
          // Wave 1A scaffold: keep validation reports in build output but
          // do not break the build while other waves are authoring content.
          failOnError: false,
          errorOnRelativeLinks: false,
          errorOnInvalidHashes: false,
          exclude: ["/reference/api", "/reference/api/**"],
        }),
        starlightOpenAPI([
          {
            base: "reference/api",
            label: "REST API",
            schema: "./openapi/simpledeploy.yaml",
          },
        ]),
      ],
      sidebar: [
        {
          label: "Start Here",
          items: [
            { label: "What is SimpleDeploy", link: "/start/what-is/" },
            { label: "How it Works", link: "/start/how-it-works/" },
            { label: "When to Use", link: "/start/when-to-use/" },
            { label: "5-Minute Quickstart", link: "/start/quickstart/" },
          ],
        },
        {
          label: "Install",
          items: [
            { label: "Overview", link: "/install/" },
            { label: "macOS (Homebrew)", link: "/install/macos/" },
            { label: "Ubuntu / Debian (apt)", link: "/install/ubuntu/" },
            { label: "Generic Linux (binary)", link: "/install/linux/" },
            { label: "From Source", link: "/install/from-source/" },
            { label: "Docker", link: "/install/docker/" },
            { label: "Upgrading", link: "/install/upgrading/" },
          ],
        },
        {
          label: "Deploy Your First App",
          items: [
            { label: "Prepare the Server", link: "/first-deploy/prepare/" },
            { label: "Generate Config", link: "/first-deploy/config/" },
            { label: "Create Admin User", link: "/first-deploy/admin/" },
            { label: "Write a docker-compose.yml", link: "/first-deploy/compose/" },
            { label: "Deploy via UI", link: "/first-deploy/ui/" },
            { label: "Deploy via CLI", link: "/first-deploy/cli/" },
            { label: "Verify", link: "/first-deploy/verify/" },
          ],
        },
        {
          label: "Core Concepts",
          autogenerate: { directory: "concepts" },
        },
        {
          label: "Guides",
          autogenerate: { directory: "guides" },
        },
        {
          label: "Playground",
          items: [
            { label: "Compose Label Builder", link: "/playground/compose/" },
          ],
        },
        {
          label: "Reference",
          items: [
            { label: "Compose Labels", link: "/reference/compose-labels/" },
            { label: "Configuration", link: "/reference/configuration/" },
            { label: "CLI", link: "/reference/cli/" },
            { label: "WebSocket Endpoints", link: "/reference/websocket/" },
            { label: "Environment Variables", link: "/reference/env-vars/" },
            { label: "Ports and Firewall", link: "/reference/ports/" },
            { label: "Directory Layout", link: "/reference/directory-layout/" },
            ...openAPISidebarGroups,
          ],
        },
        {
          label: "Operations",
          autogenerate: { directory: "operations" },
        },
        {
          label: "Integrations",
          autogenerate: { directory: "integrations" },
        },
        {
          label: "Architecture",
          autogenerate: { directory: "architecture" },
        },
        {
          label: "Contributing",
          autogenerate: { directory: "contributing" },
        },
        {
          label: "Community",
          autogenerate: { directory: "community" },
        },
        {
          label: "FAQ",
          link: "/faq/",
        },
        {
          label: "Changelog",
          link: "/changelog/",
        },
        {
          label: "Legal",
          items: [
            { label: "License", link: "/license/" },
            { label: "Privacy Policy", link: "/legal/privacy/" },
            { label: "Terms of Use", link: "/legal/terms/" },
          ],
        },
      ],
    }),
  ],
});
