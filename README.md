# go-ecommerce

GoCommerce is an e-commerce application written in Go and ReactJS. The goal of this project is to show good practices and examples of some domain or technical decisions.

The whole application is split into a few major parts:

* [backend](./backend) - the backend implementation that exposes an API for the frontend
* [frontend](./frontend) - ReactJS interface for the website
* [docs](./docs) - documentation that's more high-level (ADRs, architecture diagrams, etc)

If you find anything that you can improve or add - feel free to talk about it in the [discussions](https://github.com/bkielbasa/go-ecommerce/discussions) or create a [pull request](https://github.com/bkielbasa/go-ecommerce/pulls).

The project is a very early stage so there's a lot of work to do so every contribution is welcome!


## Services

When you run everything using docker-compose, you can access the following services:

* [grafana](http://localhost:3001/)
* [prometheus](http://localhost:9091/)
* [kibana](http://localhost:5601/)
* [logstash](http://localhost:50000)
