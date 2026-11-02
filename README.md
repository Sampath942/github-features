# GitHub Badge

Create an embeddable profile-views badge for a GitHub username. Each request records a view in Supabase and returns an SVG badge showing the updated count.

## 1. What is it?

This project is a Go function for Netlify backed by Supabase. It calls the `record_view` database function, which creates a counter for a username on the first request and atomically updates the count on later requests.

When `views_count` is provided, it sets the counter to that exact value. When it is omitted, the counter is incremented by one (or starts at `1` for a new username).

## 2. How to install?

You need:

- Go 1.26.3 or later
- Node.js and npm
- A Supabase project
- The Netlify CLI

1. Clone this repository and install the Netlify CLI:

   ```sh
   npm install --global netlify-cli
   ```

2. Apply the migration in `supabase/migrations` to your Supabase project. It creates the `view_count` table, its unique username constraint, and the atomic `record_view` function.

3. Configure the function environment variables. For local development, copy `.env.example` to `.env` and set your project URL and **service role** key:

   ```text
   SUPABASE_URL=https://YOUR_PROJECT.supabase.co
   SUPABASE_SERVICE_ROLE_KEY=YOUR_SERVICE_ROLE_KEY
   ```

   Keep the service role key private. Do not commit `.env` or expose this key in client-side code.

4. Run the unit tests and start the local Netlify server:

   ```sh
   go test ./...
   netlify dev
   ```

   For deployment, connect the repository to Netlify and set both environment variables in the site settings.

## Local integration tests

The integration test sends requests through the local Netlify function and verifies that the local Supabase database persists and increments the count. Docker must be running.

1. Start the local Supabase stack. Its first run may take a few minutes to download the required images:

   ```sh
   npx supabase start --ignore-health-check
   ```

   The local Studio/metadata health checks can time out during startup even when the API is usable; this flag leaves the stack running so the integration test can verify the API path. Apply any pending local migrations:

   ```sh
   npx supabase migration up
   ```

2. Set `SUPABASE_URL` to the local API URL and `SUPABASE_SERVICE_ROLE_KEY` to the local service role key shown by `supabase status -o env` in `.env` or the environment used to launch Netlify. Use these local values rather than a hosted Supabase project's credentials. The API uses port `55521` in this repository's config.
3. In one terminal, start Netlify:

   ```sh
   netlify dev
   ```

4. In another terminal, opt in to and run the integration test:

   ```powershell
   $env:RUN_INTEGRATION_TESTS = "1"
   go test ./tests/integration
   ```

   The test uses `http://localhost:8888` by default. Set `NETLIFY_URL` to override it. The integration test is skipped during the regular `go test ./...` run unless `RUN_INTEGRATION_TESTS=1` is set.

## 3. How to use?

The function accepts a `GET` request. Its query string parameters are:

- `username` (required): the GitHub username whose profile views are being recorded.
- `views_count` (optional): a non-negative integer that replaces the stored count with that exact value. If omitted, the stored count increments by one; a new username starts at `1`.

Locally, the basic request is:

```text
http://localhost:8888/.netlify/functions/fetch-counts?username=YOUR_GITHUB_USERNAME
```

For example, to set the count to `100`:

```text
http://localhost:8888/.netlify/functions/fetch-counts?username=YOUR_GITHUB_USERNAME&views_count=100
```

For the next request, omit `views_count` to increment the count from `100` to `101`:

```text
http://localhost:8888/.netlify/functions/fetch-counts?username=YOUR_GITHUB_USERNAME
```

If you are importing/exporting a view count from another service, call the endpoint with `views_count` set to the desired count on the first request. From the next request onward, remove `views_count` from the request; otherwise each request will replace the count instead of incrementing it.

The response is an SVG image, so it can be embedded in a Markdown README using the deployed Netlify URL:

```markdown
![Profile views](https://YOUR_SITE.netlify.app/.netlify/functions/fetch-counts?username=YOUR_GITHUB_USERNAME)
```