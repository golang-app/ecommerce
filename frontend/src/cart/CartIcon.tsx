import React from "react";
import Badge from "@mui/material/Badge";
import { styled } from "@mui/system";
import ShoppingCartIcon from "@mui/icons-material/ShoppingCart";
import Popover from "@mui/material/Popover";
import Typography from "@mui/material/Typography";
import { Link } from "react-router-dom";
import { useCart } from "./useCart";

const Box = styled("div")({});
const CartIconBox = styled("button")({
  border: "none",
  background: "none",
  cursor: "pointer",
  padding: "0",
  margin: "0",
});

export function CartIcon() {
  const cart = useCart();

  const [anchorEl, setAnchorEl] =
    React.useState<HTMLButtonElement | null>(null);

  const handleClick = (event: React.MouseEvent<HTMLButtonElement>) => {
    setAnchorEl(event.currentTarget);
  };

  const handleClose = () => {
    setAnchorEl(null);
  };

  const open = Boolean(anchorEl);
  const id = open ? "simple-popover" : undefined;
  
  if (!cart) {
    return null;
  }

  return (
    <Box>
      <Badge badgeContent={cart.countItems()}>
        <CartIconBox onClick={handleClick}>
          <ShoppingCartIcon color="action" />
        </CartIconBox>
      </Badge>

      <Popover
        id={id}
        open={open}
        anchorEl={anchorEl}
        onClose={handleClose}
        anchorOrigin={{
          vertical: "bottom",
          horizontal: "left",
        }}
      >
        {cart.countItems() > 0 ? (
          <Typography sx={{ p: 2 }}>
            <Link to="/cart">Go to cart</Link>
          </Typography>
        ) : (
          <Typography sx={{ p: 2 }}>The cart is empty</Typography>
        )}
      </Popover>
    </Box>
  );
}
