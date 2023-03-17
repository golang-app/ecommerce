import product_1 from "../page/img/feature_prod_01.jpg";
import product_2 from "../page/img/feature_prod_02.jpg";
import product_3 from "../page/img/feature_prod_03.jpg";

export function FeaturedProducts() {
  return (
    <section className="bg-light">
      <div className="container py-5">
        <div className="row text-center py-3">
          <div className="col-lg-6 m-auto">
            <h1 className="h1">Featured Product</h1>
          </div>
        </div>
        <div className="row">
          <div className="col-12 col-md-4 mb-4">
            <div className="card h-100">
              <a href="shop-single.html">
                <img src={product_1} className="card-img-top" alt="..." />
              </a>
              <div className="card-body">
                <ul className="list-unstyled d-flex justify-content-between">
                  <li>
                    <i className="text-warning fa fa-star"></i>
                    <i className="text-warning fa fa-star"></i>
                    <i className="text-warning fa fa-star"></i>
                    <i className="text-muted fa fa-star"></i>
                    <i className="text-muted fa fa-star"></i>
                  </li>
                  <li className="text-muted text-right">$240.00</li>
                </ul>
                <a
                  href="shop-single.html"
                  className="h2 text-decoration-none text-dark"
                >
                  Gym Weight
                </a>
                <p className="card-text">
                  Lorem ipsum dolor sit amet, consectetur adipisicing elit. Sunt
                  in culpa qui officia deserunt.
                </p>
                <p className="text-muted">Reviews (24)</p>
              </div>
            </div>
          </div>
          <div className="col-12 col-md-4 mb-4">
            <div className="card h-100">
              <a href="shop-single.html">
                <img src={product_2} className="card-img-top" alt="..." />
              </a>
              <div className="card-body">
                <ul className="list-unstyled d-flex justify-content-between">
                  <li>
                    <i className="text-warning fa fa-star"></i>
                    <i className="text-warning fa fa-star"></i>
                    <i className="text-warning fa fa-star"></i>
                    <i className="text-muted fa fa-star"></i>
                    <i className="text-muted fa fa-star"></i>
                  </li>
                  <li className="text-muted text-right">$480.00</li>
                </ul>
                <a
                  href="shop-single.html"
                  className="h2 text-decoration-none text-dark"
                >
                  Cloud Nike Shoes
                </a>
                <p className="card-text">
                  Aenean gravida dignissim finibus. Nullam ipsum diam, posuere
                  vitae pharetra sed, commodo ullamcorper.
                </p>
                <p className="text-muted">Reviews (48)</p>
              </div>
            </div>
          </div>
          <div className="col-12 col-md-4 mb-4">
            <div className="card h-100">
              <a href="shop-single.html">
                <img src={product_3} className="card-img-top" alt="..." />
              </a>
              <div className="card-body">
                <ul className="list-unstyled d-flex justify-content-between">
                  <li>
                    <i className="text-warning fa fa-star"></i>
                    <i className="text-warning fa fa-star"></i>
                    <i className="text-warning fa fa-star"></i>
                    <i className="text-warning fa fa-star"></i>
                    <i className="text-warning fa fa-star"></i>
                  </li>
                  <li className="text-muted text-right">$360.00</li>
                </ul>
                <a
                  href="shop-single.html"
                  className="h2 text-decoration-none text-dark"
                >
                  Summer Addides Shoes
                </a>
                <p className="card-text">
                  Curabitur ac mi sit amet diam luctus porta. Phasellus pulvinar
                  sagittis diam, et scelerisque ipsum lobortis nec.
                </p>
                <p className="text-muted">Reviews (74)</p>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
