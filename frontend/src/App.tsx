import React from 'react';
import {
  BrowserRouter as Router,
  Switch,
  Route,
  Link,
} from "react-router-dom";
import './App.css';
import 'bootstrap/dist/css/bootstrap.min.css';
import {ProductsRoute, ProductListView} from './productcatalog/product';


function App() {
  return (
  <Router>
        <nav>
          <ul>
            <li>
              <Link to="/">Home</Link>
            </li>
            <li>
              <Link to="/about">About</Link>
            </li>
            <li>
              <Link to="/users">Users</Link>
            </li>
            <li>
              <Link to="/product/prod1">Product</Link>
            </li>
          </ul>
        </nav>

        <Switch>
          <Route path="/product">
            <ProductsRoute />
          </Route>
          <Route path="/">
            <ProductListView />
          </Route>
          <Route path="/about">
            <About />
          </Route>
        </Switch>
    </Router>
  );
}

export default App;

function About() {
  return <h2>About</h2>;
}

