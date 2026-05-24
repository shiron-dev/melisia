const ACCESS_JWT_HEADER = "Cf-Access-Jwt-Assertion";

export default {
  async fetch(request, env) {
    if (request.method !== "POST") {
      return jsonResponse({ error: "method not allowed" }, 405);
    }

    if (new URL(request.url).pathname !== "/update") {
      return jsonResponse({ error: "not found" }, 404);
    }

    const token = request.headers.get(ACCESS_JWT_HEADER);
    if (!token) {
      return jsonResponse({ error: "missing access token" }, 401);
    }

    let accessClaims;
    try {
      accessClaims = await verifyAccessJwt(token, env.ACCESS_TEAM_DOMAIN, env.ACCESS_AUD);
    } catch (error) {
      return jsonResponse({ error: "invalid access token", detail: error.message }, 401);
    }

    const records = JSON.parse(env.RECORDS);
    const record = records[accessClaims.common_name];
    if (!record) {
      return jsonResponse({ error: "unknown service token" }, 403);
    }

    let body;
    try {
      body = await request.json();
    } catch {
      return jsonResponse({ error: "invalid json" }, 400);
    }

    if (!isMeshIpv4(body.ip)) {
      return jsonResponse({ error: "ip must be within 100.96.0.0/12" }, 400);
    }

    const update = await fetch(
      `https://api.cloudflare.com/client/v4/zones/${record.zone_id}/dns_records/${record.record_id}`,
      {
        method: "PATCH",
        headers: {
          Authorization: `Bearer ${env.CLOUDFLARE_API_TOKEN}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          type: "A",
          name: record.hostname,
          content: body.ip,
          ttl: 1,
          proxied: false,
          comment: "Cloudflare Mesh IP updated by mesh-dns-updater",
        }),
      },
    );

    const updateBody = await update.json().catch(() => ({}));
    if (!update.ok || !updateBody.success) {
      return jsonResponse({ error: "dns update failed", cloudflare: updateBody }, 502);
    }

    return jsonResponse({
      ok: true,
      hostname: record.hostname,
      ip: body.ip,
    });
  },
};

async function verifyAccessJwt(token, teamDomain, aud) {
  const [encodedHeader, encodedPayload, encodedSignature] = token.split(".");
  if (!encodedHeader || !encodedPayload || !encodedSignature) {
    throw new Error("malformed jwt");
  }

  const header = JSON.parse(base64UrlDecodeToString(encodedHeader));
  const payload = JSON.parse(base64UrlDecodeToString(encodedPayload));

  if (header.alg !== "RS256") {
    throw new Error("unexpected jwt algorithm");
  }

  if (!hasAudience(payload.aud, aud)) {
    throw new Error("unexpected audience");
  }

  if (payload.iss !== `https://${teamDomain}`) {
    throw new Error("unexpected issuer");
  }

  if (!payload.common_name) {
    throw new Error("missing service token common_name");
  }

  const now = Math.floor(Date.now() / 1000);
  if (payload.exp <= now) {
    throw new Error("expired token");
  }

  if (payload.nbf && payload.nbf > now) {
    throw new Error("token is not active yet");
  }

  const jwk = await findAccessJwk(teamDomain, header.kid);
  const key = await crypto.subtle.importKey(
    "jwk",
    jwk,
    { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" },
    false,
    ["verify"],
  );

  const valid = await crypto.subtle.verify(
    "RSASSA-PKCS1-v1_5",
    key,
    base64UrlDecodeToBytes(encodedSignature),
    new TextEncoder().encode(`${encodedHeader}.${encodedPayload}`),
  );

  if (!valid) {
    throw new Error("invalid signature");
  }

  return payload;
}

function hasAudience(actual, expected) {
  if (Array.isArray(actual)) {
    return actual.includes(expected);
  }
  return actual === expected;
}

async function findAccessJwk(teamDomain, kid) {
  const response = await fetch(`https://${teamDomain}/cdn-cgi/access/certs`);
  if (!response.ok) {
    throw new Error("failed to fetch access jwks");
  }

  const jwks = await response.json();
  const jwk = jwks.keys.find((key) => key.kid === kid);
  if (!jwk) {
    throw new Error("unknown access key id");
  }
  return jwk;
}

function isMeshIpv4(value) {
  if (typeof value !== "string") {
    return false;
  }

  const parts = value.split(".");
  if (parts.length !== 4) {
    return false;
  }

  const octets = parts.map((part) => {
    if (!/^(0|[1-9]\d*)$/.test(part)) {
      return Number.NaN;
    }
    return Number(part);
  });

  if (octets.some((octet) => !Number.isInteger(octet) || octet < 0 || octet > 255)) {
    return false;
  }

  return octets[0] === 100 && octets[1] >= 96 && octets[1] <= 111;
}

function base64UrlDecodeToString(value) {
  return new TextDecoder().decode(base64UrlDecodeToBytes(value));
}

function base64UrlDecodeToBytes(value) {
  const base64 = value.replace(/-/g, "+").replace(/_/g, "/").padEnd(Math.ceil(value.length / 4) * 4, "=");
  const binary = atob(base64);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes;
}

function jsonResponse(body, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: {
      "Content-Type": "application/json",
    },
  });
}
