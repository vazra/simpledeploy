package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

func (s *Server) handleTrustPage(w http.ResponseWriter, r *http.Request) {
	if s.tlsMode != "local" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, trustPageHTML)
}

func (s *Server) handleCACert(w http.ResponseWriter, r *http.Request) {
	if s.tlsMode != "local" {
		http.NotFound(w, r)
		return
	}
	caPath := filepath.Join(s.dataDir, "caddy", "pki", "authorities", "local", "root.crt")
	data, err := os.ReadFile(caPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", `attachment; filename="simpledeploy-ca.crt"`)
	w.Write(data)
}

const trustPageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>SimpleDeploy - Install Root Certificate</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #0f1117; color: #c9d1d9; line-height: 1.6; padding: 2rem 1rem; }
  .container { max-width: 640px; margin: 0 auto; }
  h1 { font-size: 1.5rem; color: #e6edf3; margin-bottom: 0.5rem; }
  .subtitle { color: #8b949e; margin-bottom: 2rem; font-size: 0.9rem; }
  .download-btn { display: inline-block; background: #238636; color: #fff; padding: 0.75rem 1.5rem; border-radius: 8px; text-decoration: none; font-weight: 600; font-size: 0.95rem; margin-bottom: 2rem; transition: background 0.2s; }
  .download-btn:hover { background: #2ea043; }
  .warning { background: #1c1c00; border: 1px solid #d29922; border-radius: 8px; padding: 1rem; margin-bottom: 2rem; font-size: 0.85rem; color: #d29922; }
  details { background: #161b22; border: 1px solid #30363d; border-radius: 8px; margin-bottom: 0.75rem; }
  summary { padding: 0.75rem 1rem; cursor: pointer; font-weight: 600; color: #e6edf3; font-size: 0.9rem; }
  summary:hover { background: #1c2129; }
  .steps { padding: 0 1rem 1rem; }
  .steps ol { padding-left: 1.25rem; }
  .steps li { margin-bottom: 0.5rem; font-size: 0.85rem; }
  code { background: #1c2129; padding: 0.15rem 0.4rem; border-radius: 4px; font-size: 0.8rem; color: #79c0ff; }
  .note { font-size: 0.8rem; color: #8b949e; margin-top: 2rem; }
</style>
</head>
<body>
<div class="container">
  <h1>Install SimpleDeploy Root Certificate</h1>
  <p class="subtitle">SimpleDeploy uses a local Certificate Authority to provide HTTPS on your network. Install the root certificate so your browser trusts these connections.</p>

  <div class="warning">This root certificate is only for your local SimpleDeploy instance. Only install it on devices you control and trust on your network.</div>

  <a href="/api/tls/ca.crt" class="download-btn">Download Root Certificate</a>

  <details>
    <summary>macOS</summary>
    <div class="steps">
      <ol>
        <li>Download the certificate file above</li>
        <li>Double-click <code>simpledeploy-ca.crt</code>. The "Add Certificates" dialog appears</li>
        <li>In the <strong>Keychain</strong> dropdown, pick <strong>login</strong> (or <strong>System</strong> for all users). <strong>Do not pick iCloud</strong>: it rejects root CAs and returns error <code>-25294</code></li>
        <li>Click <strong>Add</strong>, enter your password if prompted</li>
        <li>Open Keychain Access, find the cert (search "SimpleDeploy" or "Caddy Local Authority"), double-click it</li>
        <li>Expand <strong>Trust</strong>, set "When using this certificate" to <strong>Always Trust</strong>, close, enter your password</li>
        <li>Restart your browser</li>
      </ol>
      <p style="margin-top: 0.75rem; font-size: 0.85rem; color: #8b949e;"><strong>If the GUI still fails with <code>-25294</code>:</strong> run <code>sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain ~/Downloads/simpledeploy-ca.crt</code> in Terminal. To remove later: <code>sudo security delete-certificate -c "Caddy Local Authority" /Library/Keychains/System.keychain</code></p>
    </div>
  </details>

  <details>
    <summary>Windows</summary>
    <div class="steps">
      <ol>
        <li>Download the certificate file above</li>
        <li>Double-click <code>simpledeploy-ca.crt</code></li>
        <li>Click <strong>Install Certificate</strong></li>
        <li>Select <strong>Local Machine</strong>, click Next</li>
        <li>Select <strong>Place all certificates in the following store</strong></li>
        <li>Click Browse, select <strong>Trusted Root Certification Authorities</strong></li>
        <li>Click Next, then Finish</li>
      </ol>
    </div>
  </details>

  <details>
    <summary>Linux</summary>
    <div class="steps">
      <ol>
        <li>Download the certificate file above</li>
        <li>Copy to trust store: <code>sudo cp simpledeploy-ca.crt /usr/local/share/ca-certificates/</code></li>
        <li>Update certificates: <code>sudo update-ca-certificates</code></li>
        <li>Restart your browser</li>
      </ol>
      <p style="margin-top: 0.5rem; font-size: 0.8rem; color: #8b949e;">On Fedora/RHEL: copy to <code>/etc/pki/ca-trust/source/anchors/</code> and run <code>sudo update-ca-trust</code></p>
    </div>
  </details>

  <details>
    <summary>iOS / iPadOS</summary>
    <div class="steps">
      <ol>
        <li>Open this page in Safari on your device</li>
        <li>Tap the download button above</li>
        <li>Go to <strong>Settings &gt; General &gt; VPN &amp; Device Management</strong></li>
        <li>Tap the downloaded profile and install it</li>
        <li>Go to <strong>Settings &gt; General &gt; About &gt; Certificate Trust Settings</strong></li>
        <li>Enable full trust for the SimpleDeploy root certificate</li>
      </ol>
    </div>
  </details>

  <details>
    <summary>Android</summary>
    <div class="steps">
      <ol>
        <li>Download the certificate file above</li>
        <li>Go to <strong>Settings &gt; Security &gt; Encryption &amp; credentials</strong></li>
        <li>Tap <strong>Install a certificate &gt; CA certificate</strong></li>
        <li>Select the downloaded file</li>
        <li>Confirm installation</li>
      </ol>
      <p style="margin-top: 0.5rem; font-size: 0.8rem; color: #8b949e;">Path may vary by device manufacturer. Look for "Install certificates" in Settings search.</p>
    </div>
  </details>

  <p class="note">After installing, restart your browser. HTTPS connections to SimpleDeploy-managed domains should now work without warnings.</p>
</div>
</body>
</html>`
