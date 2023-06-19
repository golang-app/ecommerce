import { ApolloServer } from '@apollo/server';
import { startStandaloneServer } from '@apollo/server/standalone';
import { ProductsDS } from './datasource/Product.js';
import { CartDS } from './datasource/Cart.js';

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

  type Cart {
      items: [CartItem]
  }

  type CartItem {
    id: String!
    name: String!
    price: Price!
    quantity: Int!
  }

  type Query {
    listProducts: [Product]
    product(id: String!): Product
    cart(id: String!): Cart
  }

  input AddToCartInput {
      cartId: String!
      productId: String!
      quantity: Int!
  }

  type Mutation {
    addToCart(input: AddToCartInput!): String
  }

`;

interface ContextValue {
  dataSources: {
    productsAPI: ProductsDS;
    cartAPI: CartDS;
  };
}

const resolvers = {
  Query: {
    listProducts: async (_1: any, _2 : any, context : ContextValue) => {
      return context.dataSources.productsAPI.featuredProducts();
    },
    product: async (_1: any, params: any, context : ContextValue) => {
      return context.dataSources.productsAPI.product(params.id);
    },
    cart: async (_1: any, params: any, context : ContextValue) => {
      return context.dataSources.cartAPI.cart(params.id);
    },
  },
  Mutation: {
    addToCart: async (_1: any, params: any, context : ContextValue) => {
        return context.dataSources.cartAPI.addToCart(params.input);
    },
    }
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
            cartAPI: new CartDS({ cache }),
          },
        };
    },
});

console.log(`🚀  Server ready at: ${url}`);
