# Introduce GraphQL

## Context

GraphQL is a popular backend for frontend solution. As the project continues to evolve, the number of endpoints from the backend system has increased, and there is a plan to split them into separate services in the future. This creates a complexity for frontend developers to fetch data from multiple places and increases the number of requests sent to the server, which may lead to a degradation in performance.

Different Bounded Contexts will not have all information that we want to display on the frontend. It means, we have to introduce an additional layer that will gather those additional data from other Bounded Contexts. We can put them into HTTP handlers but the disadvantage of this approach is the fact that this responsibility will have to be spread in many places in the code.

## Decision

We will introduce GraphQL to the project as a layer sitting in front of all our separate backend services.

### Advantages

* Single Endpoint: Instead of multiple endpoints from different services, GraphQL provides a single endpoint to fetch and manipulate data.
* Efficiency: With GraphQL, the client can specify exactly what data it needs, which reduces the amount of data that needs to be transferred over the network and improves performance.
* Strong Typing: GraphQL is strongly typed. This can improve the quality of our code, help with tooling, and make our API self-documenting.
* Real-time data: GraphQL has built-in real-time data updates with subscriptions, which could be very beneficial for our project if we want to introduce real-time features in the future.
* Learning opportunity: It provides a good chance for the team to learn a new technology that is widely adopted in the industry.


### Disadvantages

* Learning Curve: There is a learning curve associated with GraphQL for those who are unfamiliar with it, and it could take time for the team to become proficient.
* Performance Overhead: Depending on the implementation, there might be performance overhead in resolving complex queries and managing cache.
* Over-fetching: While GraphQL allows for specifying exactly what data to fetch, this can also lead to more complex queries that return more data than needed if not properly managed.

## Alternatives

### REST APIs
REST (Representational State Transfer) APIs have been the standard for web-based data communication for a long time and are widely supported across various programming languages and platforms.

#### Pros
* **Mature and Well-understood**: REST has been around for a long time and is well-understood by many developers. There is a vast amount of documentation, tools, and libraries available.
* **Built-in HTTP Features**: REST can take advantage of HTTP features such as caching, status codes, and methods (GET, POST, PUT, DELETE, etc.).
* **Easy to Test**: Due to its simplicity and the availability of various tools, REST APIs are easy to test.
* **Versioning**: REST allows for versioning, which can help manage changes over time without breaking existing clients.

#### Cons
* **Over-fetching/Under-fetching**: REST APIs often result in either over-fetching or under-fetching of data, as the server defines what data is returned for each endpoint.
* **Multiple Round Trips**: A single client request might require multiple round trips to different endpoints to gather all necessary data.
* **Poor Real-time Capability**: REST isn't the best fit for real-time applications as it requires polling to get updates.

### gRPC
gRPC is a high-performance, open-source, universal RPC framework developed by Google.

##### Pros
* **Performance**: gRPC uses Protobuf by default for message serialization, resulting in smaller payloads and faster processing compared to JSON used in REST and GraphQL.
* **HTTP/2**: gRPC leverages HTTP/2 features such as bi-directional streaming and flow control.
* **Strongly-typed**: Like GraphQL, gRPC is strongly typed, which makes APIs self-documenting and can reduce runtime errors.

##### Cons
* **Learning Curve**: Developers new to gRPC will need to learn about Protobuf, RPC patterns, and potentially new programming languages or tools.
* **Limited Browser Support**: gRPC isn't fully supported in browsers and it doesn't work natively with JSON, which could limit its use for frontend applications.
* **Poor RESTful Support**: gRPC isn't designed to build RESTful APIs. It lacks features such as user-friendly URLs, status codes, and browser cache compatibility.

### JSON:API
JSON:API is a specification for building APIs using JSON, designed to minimize requests and data transmission.

#### Pros
* **Efficient Data Loading**: JSON:API can reduce the number of requests made by a client and limit data returned to what's necessary, much like GraphQL.
* **Standards-based**: As a specification, JSON:API provides guidance for error handling, metadata, and other concerns, which can improve consistency across APIs.

#### Cons
* **Complexity**: JSON:API's conventions can be complex to understand and implement correctly, especially compared to more straightforward options like REST.
* **Limited Flexibility**: JSON:API is more rigid in its structure compared to GraphQL. While this can have advantages, it may not suit every use case.


## Monitoring

We will leverage the ELK Stack (Elasticsearch, Logstash, Kibana) in conjunction with OpenTelemetry (OTEL) for efficient monitoring and metrics collection of our GraphQL implementation, in both development and production environments.

**Elasticsearch** will serve as the primary data store for our log data and metrics, including key measures like query execution times, error rates, and usage patterns.

**Logstash** will act as our data processing pipeline. It will collect logs from the GraphQL server, which may include resolver times and query details, transform the data as necessary, and send it to Elasticsearch.

**Kibana** will be used to visualize the data stored in Elasticsearch. It will display real-time dashboards, tracking metrics such as query latency, error rates, and usage patterns, providing valuable insights into the performance and utilization of our GraphQL layer.

**OpenTelemetry** (OTEL) will be integrated into our setup to standardize the collection and distribution of telemetry data (traces, metrics, logs) from our GraphQL service to the ELK Stack, thus enhancing observability.

## Consequences
If successful, this decision will lead to easier data management, improved performance, and a streamlined API. It will also provide the team with valuable experience in a new technology. If not, we may need to invest more time in understanding the intricacies of GraphQL and the potential performance overheads.
