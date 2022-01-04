import React from "react";
import { BrowserRouter as Router, Switch, Route } from "react-router-dom";
import "./App.css";
import "bootstrap/dist/css/bootstrap.min.css";
import { ProductsRoute, ProductListView } from "./productcatalog/product";
import { Nav, Navbar, Container } from "react-bootstrap";

function App() {
  return (
    <Router>
      <Navbar>
        <Container>
          <Nav>
            <Nav.Link href="/">Home</Nav.Link>
            <Nav.Link href="/about">About</Nav.Link>
            <Nav.Link href="/users">Users</Nav.Link>
          </Nav>
        </Container>
      </Navbar>

      <Switch>
        <Route path="/product">
          <ProductsRoute />
        </Route>
        <Route path="/">
          <ProductListView />
        </Route>
      </Switch>
    </Router>
  );
}

export default App;
