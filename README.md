# go-ecommerce

GoCommerce is an e-commerce application written in Go and ReactJS. The goal of this project is to show good practices and examples of some domain or technical decisions.

The whole application is split into a few major parts:

* [backend](./backend) - the backend implementation that exposes an API for the frontend with frontend written in HTMX
* [docs](./docs) - documentation that's more high-level (ADRs, architecture diagrams, etc)

If you find anything that you can improve or add - feel free to talk about it in the [discussions](https://github.com/bkielbasa/go-ecommerce/discussions) or create a [pull request](https://github.com/bkielbasa/go-ecommerce/pulls).

The project is a very early stage so there's a lot of work to do so every contribution is welcome!


## Quick start

The easiest way of running everything is using the `docker-compose`.

```sh
docker-compose up
```

You'll have to wait some time to download all dependencies and build everything but after it, everything should be up and running.
