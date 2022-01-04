import React from "react";
import { Switch, Route, Link, useRouteMatch } from "react-router-dom";
import Col from "react-bootstrap/Col";
import Image from "react-bootstrap/Image";
import Container from "react-bootstrap/Container";
import Row from "react-bootstrap/Row";

export interface Product {
  id: string;
  name: string;
  thumbnail: string;
  price: Price;
}

interface Price {
  currency: string;
  amount: number;
}

function SmallProductView(product: Product) {
  return (
    <Col className="product-view">
      <div className="thumbnail">
        <Image src={product.thumbnail} thumbnail />
      </div>
      <div className="name">
        <h2>
          <Link to={`/product/${product.id}`}>{product.name}</Link>
        </h2>
      </div>
      <div className="price">
        Price: {product.price.amount} {product.price.currency}
      </div>
    </Col>
  );
}

export function ProductsRoute() {
  let match = useRouteMatch();

  return (
    <Switch>
      <Route exact path={`${match.path}/:productId`} component={ShowProduct} />
      <Route path={match.path}>
        <ProductListView />
      </Route>
    </Switch>
  );
}

interface ProductListViewProps {}

interface ProductListViewState {
  error: string;
  isLoaded: boolean;
  products: Product[];
}

export class ProductListView extends React.Component<
  ProductListViewProps,
  ProductListViewState
> {
  constructor(props: any) {
    super(props);

    this.state = {
      error: "",
      isLoaded: false,
      products: [],
    };
  }

  componentDidMount() {
    fetch("http://localhost:8080/products")
      .then((res) => res.json())
      .then(
        (result) => {
          this.setState({
            isLoaded: true,
            products: result.Products,
            error: "",
          });
        },
        (error) => {
          this.setState({
            isLoaded: true,
            error,
          });
        }
      );
  }

  render() {
    const { error, isLoaded, products } = this.state;

    if (error) {
      return <div>Error: {error}</div>;
    } else if (!isLoaded) {
      return <div>Loading...</div>;
    } else {
      const renderProducts = () => {
        return products.map((product) => {
          return <SmallProductView key={product.id} {...product} />;
        });
      };
      return (
        <Container>
          <Row>{renderProducts()}</Row>
        </Container>
      );
    }
  }
}

interface ShowProductProps {
  productID: string;
}

interface ShowProductState {
  error: string;
  isLoaded: boolean;
  product?: Product;
}

export class ShowProduct extends React.Component<
  ShowProductProps,
  ShowProductState
> {
  productID: string;

  constructor(props: any) {
    super(props);
    this.productID = props.match.params.productId;

    this.state = {
      error: "",
      isLoaded: false,
      product: undefined,
    };
  }

  componentDidMount() {
    fetch("http://localhost:8080/product/" + this.productID)
      .then((res) => res.json())
      .then(
        (result) => {
          this.setState({
            isLoaded: true,
            product: result,
            error: "",
          });
        },
        (error) => {
          this.setState({
            isLoaded: true,
            error,
          });
        }
      );
  }

  render() {
    const { error, isLoaded, product } = this.state;

    if (error) {
      return <div>Error: {error}</div>;
    } else if (!isLoaded || product === undefined) {
      return <div>Loading...</div>;
    } else {
      return (
        <div>
          <h3>{product.name}</h3>
          <p>
            {product.price.amount} {product.price.currency}
          </p>
          <p>
            <img src={product.thumbnail} alt={product.name} width="300px" />
          </p>
        </div>
      );
    }
  }
}
