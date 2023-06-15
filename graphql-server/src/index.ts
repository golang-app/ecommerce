import { ApolloServer } from '@apollo/server';
import { startStandaloneServer } from '@apollo/server/standalone';
import { ProductsDS } from './datasource/Product.js';

const Query = `
  type Product {
    id: String!
    name: String!
    description: String
    price: Price
  }

  type Price {
      amount: Int!
      currency: String!
  }

  type Query {
    featuredProducts: [Product]
    product(id: String!): Product
  }
`;

interface ContextValue {
  dataSources: {
    productsAPI: ProductsDS;
  };
}

const resolvers = {
  Query: {
    featuredProducts: async (_1: any, _2 : any, context : ContextValue) => {
      return context.dataSources.productsAPI.featuredProducts();
    },
    product: async (_1: any, params: any, context : ContextValue) => {
      return context.dataSources.productsAPI.product(params.id);
    },
  },
};


const server = new ApolloServer<ContextValue>({
 typeDefs: [ Query],
  resolvers,
});

const { url } = await startStandaloneServer(server, {
    listen: { port: 4000 },
    context: async () => {
        const { cache } = server;

        return {
          dataSources: {
            productsAPI: new ProductsDS({ cache }),
          },
        };
    },
});

console.log(`🚀  Server ready at: ${url}`);
