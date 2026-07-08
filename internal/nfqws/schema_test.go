package nfqws

import (
	"strings"
	"testing"
)

const sampleConf = `# Provider network interface
ISP_INTERFACE="eth3"

# HTTP(S) strategy
NFQWS_ARGS="--filter-tcp=443,80 --filter-l7=http,tls
            # HTTPS
            --lua-desync=multisplit:pos=1,midsld:strategy=1"

MODE_AUTO="$MODE_LIST --hostlist-auto=/opt/etc/nfqws2/lists/auto.list $MODE_ALL"
NFQWS_EXTRA_ARGS="$MODE_AUTO"

NFQWS_ARGS_CUSTOM=""
IPV6_ENABLED=1
TCP_PORTS=80,443,1984
UDP_PORTS=443,590:600
POLICY_NAME="nfqws"
POLICY_EXCLUDE=0
LOG_LEVEL=0
NFQUEUE_NUM=300
`

func TestParseConf(t *testing.T) {
	c, err := ParseConf(sampleConf)
	if err != nil {
		t.Fatal(err)
	}
	if c.ISPInterface != "eth3" {
		t.Fatalf("ISP_INTERFACE=%q", c.ISPInterface)
	}
	if c.TCPPorts != "80,443,1984" {
		t.Fatalf("TCP_PORTS=%q", c.TCPPorts)
	}
	if c.UDPPorts != "443,590:600" {
		t.Fatalf("UDP_PORTS=%q", c.UDPPorts)
	}
	if c.PolicyName != "nfqws" || c.PolicyExclude != 0 {
		t.Fatalf("policy=%q/%d", c.PolicyName, c.PolicyExclude)
	}
	if c.NfqueueNum != 300 {
		t.Fatalf("NFQUEUE_NUM=%d", c.NfqueueNum)
	}
	if !c.IPv6Enabled {
		t.Fatal("IPV6_ENABLED should parse true")
	}
	if c.NfqwsExtraArgs != "$MODE_AUTO" {
		t.Fatalf("NFQWS_EXTRA_ARGS=%q", c.NfqwsExtraArgs)
	}
	if !strings.Contains(c.NfqwsArgs, "multisplit") || !strings.Contains(c.NfqwsArgs, "# HTTPS") {
		t.Fatalf("multiline NFQWS_ARGS not captured: %q", c.NfqwsArgs)
	}
}

func TestRenderInPlace(t *testing.T) {
	c, _ := ParseConf(sampleConf)
	c.TCPPorts = "80,443"
	c.IPv6Enabled = false
	c.PolicyName = "myproxy"

	out := c.Render(sampleConf)

	if !strings.Contains(out, "TCP_PORTS=80,443\n") {
		t.Fatalf("TCP_PORTS not updated:\n%s", out)
	}
	if !strings.Contains(out, "IPV6_ENABLED=0") {
		t.Fatalf("IPV6_ENABLED not updated:\n%s", out)
	}
	if !strings.Contains(out, `POLICY_NAME="myproxy"`) {
		t.Fatalf("POLICY_NAME not updated:\n%s", out)
	}
	// Untouched multiline strategy + comments preserved verbatim.
	if !strings.Contains(out, "            # HTTPS\n") {
		t.Fatalf("multiline inner comment lost:\n%s", out)
	}
	if !strings.Contains(out, "--lua-desync=multisplit:pos=1,midsld:strategy=1") {
		t.Fatal("strategy body lost")
	}
	if !strings.Contains(out, "# Provider network interface") {
		t.Fatal("leading comment lost")
	}

	c2, _ := ParseConf(out)
	if c2.TCPPorts != "80,443" || c2.IPv6Enabled || c2.PolicyName != "myproxy" {
		t.Fatalf("round-trip mismatch: %+v", c2)
	}
	if c2.NfqwsArgs != c.NfqwsArgs {
		t.Fatalf("multiline value changed across round-trip")
	}
}

func TestRenderNoChangeIsLossless(t *testing.T) {
	c, _ := ParseConf(sampleConf)
	if out := c.Render(sampleConf); out != sampleConf {
		t.Fatalf("no-op render must be byte-identical:\n--- got ---\n%q\n--- want ---\n%q", out, sampleConf)
	}
}
