import sys, json
from playwright.sync_api import sync_playwright
url = sys.argv[1]
with sync_playwright() as p:
    b = p.chromium.launch(channel="chrome", headless=True)
    pg = b.new_page(viewport={"width": 1400, "height": 900})
    errors, failed = [], []
    pg.on("console", lambda m: errors.append(f"{m.type}: {m.text}") if m.type in ("error", "warning") else None)
    pg.on("pageerror", lambda e: errors.append(f"pageerror: {e}"))
    pg.on("requestfailed", lambda r: failed.append(f"{r.url} {r.failure}"))
    pg.on("response", lambda r: failed.append(f"{r.status} {r.url}") if r.status >= 400 else None)
    pg.goto(url, wait_until="networkidle", timeout=60000)
    pg.wait_for_timeout(4000)
    body_text = pg.inner_text("body")[:400].replace("\n", " | ")
    print("title:", pg.title())
    print("body text:", body_text or "<EMPTY>")
    print("errors:", json.dumps(errors[:15], indent=1))
    print("failed/4xx:", json.dumps(failed[:15], indent=1))
    pg.screenshot(path="home.png", full_page=False)
    b.close()
