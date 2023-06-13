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

* Tooling: While GraphQL has matured significantly, it's not as ubiquitous as REST. Certain tooling, middleware or libraries the team relies on might not have equivalent support in the GraphQL ecosystem.


## Consequences
If successful, this decision will lead to easier data management, improved performance, and a streamlined API. It will also provide the team with valuable experience in a new technology. If not, we may need to invest more time in understanding the intricacies of GraphQL and the potential performance overheads.
