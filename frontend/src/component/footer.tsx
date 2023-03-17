import Container from "react-bootstrap/Container";
import Row from "react-bootstrap/Row";
import Col from "react-bootstrap/Col";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import {
  faFacebook,
  faInstagram,
  faTwitter,
  faLinkedin,
} from "@fortawesome/free-brands-svg-icons";
import "./footer.css";

export function Footer() {
  return (
    <footer className="bg-dark" id="tempaltemo_footer">
      <Container>
        <Row>
          <Col md={3} className="col">
            <h2 className="h2 text-success border-bottom pb-3 border-light logo">
              Zay Shop
            </h2>
            <ul className="list-unstyled text-light footer-link-list">
              <li>
                <i className="fas fa-map-marker-alt fa-fw"></i>
                123 Consectetur at ligula 10660
              </li>
              <li>
                <i className="fa fa-phone fa-fw"></i>
                <a className="text-decoration-none" href="tel:010-020-0340">
                  010-020-0340
                </a>
              </li>
              <li>
                <i className="fa fa-envelope fa-fw"></i>
                <a
                  className="text-decoration-none"
                  href="mailto:info@company.com"
                >
                  info@company.com
                </a>
              </li>
            </ul>
          </Col>

          <Col md={3} className="col">
            <h2 className="h2 text-light border-bottom pb-3 border-light">
              Products
            </h2>
            <ul className="list-unstyled text-light footer-link-list">
              <li>
                <a className="text-decoration-none" href="#">
                  Luxury
                </a>
              </li>
              <li>
                <a className="text-decoration-none" href="#">
                  Sport Wear
                </a>
              </li>
              <li>
                <a className="text-decoration-none" href="#">
                  Men's Shoes
                </a>
              </li>
              <li>
                <a className="text-decoration-none" href="#">
                  Women's Shoes
                </a>
              </li>
              <li>
                <a className="text-decoration-none" href="#">
                  Popular Dress
                </a>
              </li>
              <li>
                <a className="text-decoration-none" href="#">
                  Gym Accessories
                </a>
              </li>
              <li>
                <a className="text-decoration-none" href="#">
                  Sport Shoes
                </a>
              </li>
            </ul>
          </Col>

          <Col md={3} className="col">
            <h2 className="h2 text-light border-bottom pb-3 border-light">
              Further Info
            </h2>
            <ul className="list-unstyled text-light footer-link-list">
              <li>
                <a className="text-decoration-none" href="#">
                  Home
                </a>
              </li>
              <li>
                <a className="text-decoration-none" href="#">
                  About Us
                </a>
              </li>
              <li>
                <a className="text-decoration-none" href="#">
                  Shop Locations
                </a>
              </li>
              <li>
                <a className="text-decoration-none" href="#">
                  FAQs
                </a>
              </li>
              <li>
                <a className="text-decoration-none" href="#">
                  Contact
                </a>
              </li>
            </ul>
          </Col>
        </Row>

        <Row>
          <div className="col-12 mb-3">
            <div className="w-100 my-3 border-top border-light"></div>
          </div>
          <div className="col-auto me-auto">
            <ul className="list-inline text-left footer-icons">
              <li className="list-inline-item border border-light rounded-circle text-center">
                <a
                  className="text-light text-decoration-none"
                  target="_blank"
                  href="http://facebook.com/"
                >
                  <FontAwesomeIcon icon={faFacebook} />
                </a>
              </li>
              <li className="list-inline-item border border-light rounded-circle text-center">
                <a
                  className="text-light text-decoration-none"
                  target="_blank"
                  href="https://www.instagram.com/"
                >
                  <FontAwesomeIcon icon={faInstagram} />
                </a>
              </li>
              <li className="list-inline-item border border-light rounded-circle text-center">
                <a
                  className="text-light text-decoration-none"
                  target="_blank"
                  href="https://twitter.com/"
                >
                  <FontAwesomeIcon icon={faTwitter} />
                </a>
              </li>
              <li className="list-inline-item border border-light rounded-circle text-center">
                <a
                  className="text-light text-decoration-none"
                  target="_blank"
                  href="https://www.linkedin.com/"
                >
                  <FontAwesomeIcon icon={faLinkedin} />
                </a>
              </li>
            </ul>
          </div>
          <div className="col-auto">
            <label className="sr-only">Email address</label>
            <div className="input-group mb-2">
              <input
                type="text"
                className="form-control bg-dark border-light"
                id="subscribeEmail"
                placeholder="Email address"
              />
              <div className="input-group-text btn-success text-light">
                Subscribe
              </div>
            </div>
          </div>
        </Row>
      </Container>

      <div className="w-100 bg-black py-3">
        <Container>
          <Row>
            <div className="col-12">
              <p className="text-left text-light">
                Copyright &copy; 2021 Company Name | Designed by{" "}
                <a
                  rel="sponsored"
                  href="https://templatemo.com"
                  target="_blank"
                >
                  TemplateMo
                </a>
              </p>
            </div>
          </Row>
        </Container>
      </div>
    </footer>
  );
}
