# Gitlab Gogot (gg)
-

I'm not sure where it went wrong but it used to be that an authenticated token could go get a project in a subgroup if it had access to that group, but it seems that might have changed.

This tiny tool restores that functionality.

## Usage

### Help

```
-api string
	gitlab api endpoint {GG_GITLAB_API} (default "https://gitlab.com/api/v4")
-cachesize int
	size of the cache  {GG_CACHE_SIZE} (default 512)
-listen string
	listen host:port {GG_LISTEN} (default "127.0.0.1:9181")
```

### Example

```
./gogot -api "https://gitlab.example.com/api/v4"
```

## Accessing

You will need to set up something in front of gitlab or modify the gitlab nginx configuration

### Caddy Hint

There might be an easier way, but meh.

```
https://git.example.com {
    tls {
        dns cloudflare
    }

    # Add goget path if go-get is in the params
    rewrite {
        if {?go-get} is "1"
        if {path} not_starts_with "/____goget"
        if_op and
        to /____goget/{path}
    }

    root /opt/www/gitlab

    proxy / http://127.0.0.1:8181 {
        except /assets
        except /____goget
        fail_timeout 0s
        transparent
        header_upstream X-Forwarded-Ssl on
    }

    proxy /____goget http://127.0.0.1:9181 {
        fail_timeout 0s
        transparent
        without /____goget
    }
}
```

## Security

This shouldn't open you to any more risk than the old method did, it still requires you to pass your gitlab token.