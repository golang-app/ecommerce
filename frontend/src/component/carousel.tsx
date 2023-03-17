import "./carousel.css";
import banner01 from "../page/img/banner_img_01.jpg";
import banner02 from "../page/img/banner_img_02.jpg";
import banner03 from "../page/img/banner_img_03.jpg";

export function Carousel() {
  return (
    <div
      id="template-mo-zay-hero-carousel"
      className="carousel slide"
      data-bs-ride="carousel"
    >
      <ol className="carousel-indicators">
        <li
          data-bs-target="#template-mo-zay-hero-carousel"
          data-bs-slide-to="0"
          className="active"
        ></li>
        <li
          data-bs-target="#template-mo-zay-hero-carousel"
          data-bs-slide-to="1"
        ></li>
        <li
          data-bs-target="#template-mo-zay-hero-carousel"
          data-bs-slide-to="2"
        ></li>
      </ol>
      <div className="carousel-inner">
        <div className="carousel-item active">
          <div className="container">
            <div className="row p-5">
              <div className="mx-auto col-md-8 col-lg-6 order-lg-last">
                <img className="img-fluid" src={banner01} alt="" />
              </div>
              <div className="col-lg-6 mb-0 d-flex align-items-center">
                <div className="text-align-left align-self-center">
                  <h1 className="h1 text-success">
                    <b>Zay</b> eCommerce
                  </h1>
                  <h3 className="h2">Tiny and Perfect eCommerce Template</h3>
                  <p>
                    Zay Shop is an eCommerce HTML5 CSS template with latest
                    version of Bootstrap 5 (beta 1). This template is 100% free
                    provided by{" "}
                    <a
                      rel="sponsored"
                      className="text-success"
                      href="https://templatemo.com"
                      target="_blank"
                    >
                      TemplateMo
                    </a>{" "}
                    website. Image credits go to{" "}
                    <a
                      rel="sponsored"
                      className="text-success"
                      href="https://stories.freepik.com/"
                      target="_blank"
                    >
                      Freepik Stories
                    </a>
                    ,
                    <a
                      rel="sponsored"
                      className="text-success"
                      href="https://unsplash.com/"
                      target="_blank"
                    >
                      Unsplash
                    </a>{" "}
                    and
                    <a
                      rel="sponsored"
                      className="text-success"
                      href="https://icons8.com/"
                      target="_blank"
                    >
                      Icons 8
                    </a>
                    .
                  </p>
                </div>
              </div>
            </div>
          </div>
        </div>
        <div className="carousel-item">
          <div className="container">
            <div className="row p-5">
              <div className="mx-auto col-md-8 col-lg-6 order-lg-last">
                <img className="img-fluid" src={banner02} alt="" />
              </div>
              <div className="col-lg-6 mb-0 d-flex align-items-center">
                <div className="text-align-left">
                  <h1 className="h1">Proident occaecat</h1>
                  <h3 className="h2">Aliquip ex ea commodo consequat</h3>
                  <p>
                    You are permitted to use this Zay CSS template for your
                    commercial websites. You are <strong>not permitted</strong>{" "}
                    to re-distribute the template ZIP file in any kind of
                    template collection websites.
                  </p>
                </div>
              </div>
            </div>
          </div>
        </div>
        <div className="carousel-item">
          <div className="container">
            <div className="row p-5">
              <div className="mx-auto col-md-8 col-lg-6 order-lg-last">
                <img className="img-fluid" src={banner03} alt="" />
              </div>
              <div className="col-lg-6 mb-0 d-flex align-items-center">
                <div className="text-align-left">
                  <h1 className="h1">Repr in voluptate</h1>
                  <h3 className="h2">Ullamco laboris nisi ut </h3>
                  <p>
                    We bring you 100% free CSS templates for your websites. If
                    you wish to support TemplateMo, please make a small
                    contribution via PayPal or tell your friends about our
                    website. Thank you.
                  </p>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
      <a
        className="carousel-control-prev text-decoration-none w-auto ps-3"
        href="#template-mo-zay-hero-carousel"
        role="button"
        data-bs-slide="prev"
      >
        <i className="fas fa-chevron-left"></i>
      </a>
      <a
        className="carousel-control-next text-decoration-none w-auto pe-3"
        href="#template-mo-zay-hero-carousel"
        role="button"
        data-bs-slide="next"
      >
        <i className="fas fa-chevron-right"></i>
      </a>
    </div>
  );
}
