export type ParsedUserAgent = {
  browser: string;
  os: string;
};

/** Tiny UA → browser/OS labels. No dependency. */
export function parseUserAgent(ua: string): ParsedUserAgent {
  const s = ua.trim();
  if (!s) return { browser: "Unknown", os: "Unknown" };

  let os = "Unknown";
  if (/Android/i.test(s)) os = "Android";
  else if (/iPhone|iPad|iPod/i.test(s)) os = "iOS";
  else if (/Windows NT/i.test(s)) os = "Windows";
  else if (/Mac OS X|Macintosh/i.test(s)) os = "macOS";
  else if (/Linux/i.test(s)) os = "Linux";

  let browser = "Unknown";
  if (/Edg\//i.test(s)) browser = "Edge";
  else if (/Chrome\//i.test(s) && !/Chromium/i.test(s)) browser = "Chrome";
  else if (/Firefox\//i.test(s)) browser = "Firefox";
  else if (/Safari\//i.test(s) && !/Chrome\//i.test(s)) browser = "Safari";

  return { browser, os };
}
