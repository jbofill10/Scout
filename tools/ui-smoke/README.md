# UI smoke probe

Loads a Scout page in headless Chrome and reports what a status code cannot:
uncaught JavaScript errors, failed requests, and whether anything rendered.
A chunk-ordering bug once left the homepage blank while every asset and API
answered 200; this catches that class of failure.

```bash
pip install playwright && playwright install chrome   # once
python3 tools/ui-smoke/probe.py http://localhost:30030/
python3 tools/ui-smoke/probe.py http://localhost:30030/activity
```

Output: the page title, the first 400 characters of visible text (`<EMPTY>`
means nothing rendered), console errors and page errors, and any 4xx/5xx or
failed requests. A screenshot lands in `home.png` in the working directory.
Run it after every UI deploy, against the real host.
