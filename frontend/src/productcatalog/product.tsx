import React from 'react';
import {
  Switch,
  Route,
  Link,
  useRouteMatch,
  useParams
} from "react-router-dom";
import Col from 'react-bootstrap/Col';
import Image from 'react-bootstrap/Image';
import Container from 'react-bootstrap/Container';
import Row from 'react-bootstrap/Row';

export interface Product{
  id: string;
  name: string;
  thumbanil: string;
  price: string;
}
 
function SmallProductView(product :Product) {
      return (
        <Col className="product-view">
          <div className="thumbnail">
            <Image src={product.thumbanil} thumbnail />
          </div>
          <div className="name">
            <h2><Link to={`/product/${product.id}`}>{product.name}</Link></h2>
          </div>
          <div className="price">
            Price: {product.price}
          </div>
        </Col>
  );
}

export function ProductsRoute() {
    let match = useRouteMatch();

    return (
    <Switch>
        <Route path={`${match.path}/:productId`}>
          <ShowProduct />
        </Route>
        <Route path={match.path}>
          <ProductListView />
        </Route>
      </Switch>
    )
}

interface ProductListViewProps {}

interface ProductListViewState {
  error: string;
  isLoaded: boolean;
  products: Product[]
}

export class ProductListView extends React.Component<ProductListViewProps, ProductListViewState> {
    constructor(props: any) {
    super(props);

    this.state = {
        error: "",
        isLoaded: false,
        products: [],
    }
  }

    render() {
    const { error, isLoaded, products } = this.state;
    if (error) {
      return <div>Error: {error}</div>;
    } else if (!isLoaded) {
      return <div>Loading...</div>;
    } else {
        const renderProducts = () => {
          return products.map(product => {
            return <SmallProductView {...product} />;
          });
        };
        return(
        <Container>
            <Row>
            {renderProducts()}
            </Row>
        </Container>
        )
    }
    }
}

function ShowProduct() {
  const { productId } = useParams() as { 
  productId: string;
}
  return <h3>Requested product ID: {productId}</h3>;
}
